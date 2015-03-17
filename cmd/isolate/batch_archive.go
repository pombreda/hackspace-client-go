// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"strings"

	"infra/libs/parallel"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"chromium.googlesource.com/infra/swarming/client-go/isolate"
	"github.com/maruel/interrupt"
	"github.com/maruel/subcommands"
)

var cmdBatchArchive = &subcommands.Command{
	UsageLine: "batcharchive file1 file2 ...",
	ShortDesc: "archives multiple isolated trees at once.",
	LongDesc: `Archives multiple isolated trees at once.

Using single command instead of multiple sequential invocations allows to cut
redundant work when isolated trees share common files (e.g. file hashes are
checked only once, their presence on the server is checked only once, and
so on).

Takes a list of paths to *.isolated.gen.json files that describe what trees to
isolate. Format of files is:
{
  "version": 1,
  "dir": <absolute path to a directory all other paths are relative to>,
  "args": [list of command line arguments for single 'archive' command]
}`,
	CommandRun: func() subcommands.CommandRun {
		c := batchArchiveRun{}
		c.commonFlags.Init(&c.CommandRunBase)
		c.commonServerFlags.Init(&c.CommandRunBase)
		c.Flags.StringVar(&c.dumpJson, "dump-json", "",
			"Write isolated Digestes of archived trees to this file as JSON")
		return &c
	},
}

type batchArchiveRun struct {
	subcommands.CommandRunBase
	commonFlags
	commonServerFlags
	dumpJson string
}

func (c *batchArchiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("at least one isolate file required")
	}
	return nil
}

func parseArchiveCMD(args []string, cwd string) (*isolate.ArchiveOptions, error) {
	// Python isolate allows form "--XXXX-variable key value".
	// Golang flag pkg doesn't consider value to be part of --XXXX-variable flag.
	// Therefore, we convert all such "--XXXX-variable key value" to
	// "--XXXX-variable key --XXXX-variable value" form.
	// Note, that key doesn't have "=" in it in either case, but value might.
	// TODO(tandrii): eventually, we want to retire this hack.
	args = convertPyToGoArchiveCMDArgs(args)
	base := subcommands.CommandRunBase{}
	i := isolateFlags{}
	i.Init(&base)
	if err := base.GetFlags().Parse(args); err != nil {
		return nil, err
	}
	if err := i.Parse(); err != nil {
		return nil, err
	}
	if base.GetFlags().NArg() > 0 {
		return nil, fmt.Errorf("no positional arguments expected")
	}
	return &i.ArchiveOptions, nil
}

// convertPyToGoArchiveCMDArgs converts kv-args from old python isolate into go variants.
// Essentially converts "--X key value" into "--X key=value".
func convertPyToGoArchiveCMDArgs(args []string) []string {
	kvars := map[string]bool{
		"--path-variable": true, "--config-variable": true, "--extra-vriable": true}
	newArgs := []string{}
	for i := 0; i < len(args); {
		newArgs = append(newArgs, args[i])
		kvar := args[i]
		i++
		if !kvars[kvar] {
			continue
		}
		if i >= len(args) {
			// Ignore unexpected behaviour, it'll be caught by flags.Parse() .
			break
		}
		appendArg := args[i]
		i++
		if !strings.Contains(appendArg, "=") && i < len(args) {
			// appendArg is key, and args[i] is value .
			appendArg = fmt.Sprintf("%s=%s", appendArg, args[i])
			i++
		}
		newArgs = append(newArgs, appendArg)
	}
	return newArgs
}

type parseGenFileResult struct {
	dir  string
	opts *isolate.ArchiveOptions
}

func parseGenFile(genJsonPath string) (parseGenFileResult, error) {
	data := struct {
		Args    []string
		Dir     string
		Version int
	}{}
	result := parseGenFileResult{}
	err := common.ReadJSONFile(genJsonPath, &data)
	if err != nil {
		return result, err
	}
	if data.Version != isolate.ISOLATED_GEN_JSON_VERSION {
		return result, fmt.Errorf("Invalid version %d in %s", data.Version, genJsonPath)
	} else if !common.IsDirectory(data.Dir) {
		return result, fmt.Errorf("Invalid dir %s in %s", data.Dir, genJsonPath)
	} else {
		result.opts, err = parseArchiveCMD(data.Args, data.Dir)
		result.dir = data.Dir
	}
	return result, err
}

func parseGenFiles(genJsonPaths []string) (<-chan isolate.Tree, <-chan error) {
	chTrees := make(chan isolate.Tree)
	chErrors := make(chan error, 1)
	go func() {
		defer close(chTrees)
		defer close(chErrors)
		chErrors <- parallel.FanOutIn(func(ch chan<- func() error) {
			for _, genJsonPath := range genJsonPaths {
				select {
				case ch <- func() error {
					if result, err := parseGenFile(genJsonPath); err != nil {
						return err
					} else {
						select {
						case chTrees <- isolate.Tree{result.dir, *result.opts}:
						case <-interrupt.Channel:
							return errors.New("interrupted")
						}
						return nil
					}
				}:
				case <-interrupt.Channel:
					return
				}
			}
		})
	}()
	return chTrees, chErrors
}

func (c *batchArchiveRun) main(a subcommands.Application, args []string) error {
	// Library interrupt is used for clean handling of Ctrl+C or in case of unrecoverable errors.
	defer interrupt.Set()
	// 3 step pipeline is connected using two channels:
	// [Parsing Gen Files] => chTrees => [Isolate] => chFileAssets => [Archive] .
	// The error channels are collected here.
	chTrees, chGenErrors := parseGenFiles(args)
	chIsolateHashes, chFileAssets, chIsoErrors := isolate.IsolateAsync(chTrees)
	chArchiveErrors := isolate.ArchiveAsync(chFileAssets, c.serverURL, c.namespace)
	select {
	case cerr := <-chGenErrors:
		if cerr != nil {
			return cerr
		}
	case ierr := <-chIsoErrors:
		if ierr != nil {
			return ierr
		}
	case uerr := <-chArchiveErrors:
		if uerr != nil {
			return uerr
		} else {
			// Success, no uploading error.
			isolatedHashes := <-chIsolateHashes
			if c.dumpJson != "" {
				return common.WriteJSONFile(c.dumpJson, isolatedHashes)
			}
			return nil
		}
	case <-interrupt.Channel:
		return errors.New("interrupted")
	}
	// Unreachable code.
	return nil
}

func (c *batchArchiveRun) Run(a subcommands.Application, args []string) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
