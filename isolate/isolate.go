// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package isolate implements the code to process '.isolate' files.
package isolate

import (
	"log"
	"regexp"
	"strconv"
	"sync"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"chromium.googlesource.com/infra/swarming/client-go/isolateserver"
)

const ISOLATED_GEN_JSON_VERSION = 1
const VALID_VARIABLE = "[A-Za-z_][A-Za-z_0-9]*"

var VALID_VARIABLE_MATCHER = regexp.MustCompile(VALID_VARIABLE)

func IsValidVariable(variable string) bool {
	return VALID_VARIABLE_MATCHER.MatchString(variable)
}

// Tree to be isolated.
type Tree struct {
	Cwd  string
	Opts ArchiveOptions
}

// ArchiveOptions for achiving trees.
type ArchiveOptions struct {
	Isolate         string            `json:"isolate"`
	Isolated        string            `json:"isolated"`
	Blacklist       []string          `json:"blacklist"`
	PathVariables   map[string]string `json:"path_variables"`
	ExtraVariables  map[string]string `json:"extra_variables"`
	ConfigVariables map[string]string `json:"config_variables"`
}

// NewArchiveOptions initializes with non-nil values
func (a *ArchiveOptions) Init() {
	a.Blacklist = []string{}
	a.PathVariables = map[string]string{}
	a.ExtraVariables = map[string]string{}
	a.ConfigVariables = map[string]string{}
}

type FileMetadata struct {
	meta map[string]string
}

func (m *FileMetadata) IsSymlink() bool {
	return m.meta["l"] != ""
}
func (m *FileMetadata) IsHighPriority() bool {
	return m.meta["priority"] == "0"
}
func (m *FileMetadata) GetDigest() string {
	return m.meta["h"]
}

func (m *FileMetadata) GetSize() (int64, error) {
	return strconv.ParseInt(m.meta["s"], 10, 64)
}

type FileAsset struct {
	*FileMetadata
	fullPath string
}

func (f *FileAsset) GetSize() int64 {
	if size, err := f.FileMetadata.GetSize(); err != nil {
		size, err = common.GetFileSize(f.fullPath)
		if err != nil {
			// TODO(tandrii): handle this error
			panic(err)
		}
		return size
	} else {
		return size
	}
}

func (fa *FileAsset) ToUploadItem() isolateserver.UploadItem {
	// TODO(tandrii): get_zip_compression_level.
	f := isolateserver.FileItem{
		isolateserver.Item{
			Digest:           fa.meta["h"],
			Size:             fa.GetSize(),
			HighPriority:     fa.IsHighPriority(),
			CompressionLevel: 6,
		},
		fa.fullPath,
	}
	return &f
}

func isolateTree(done <-chan struct{}, tree Tree, chFileAssets chan<- FileAsset) ([]string, error) {
	//return nil, fmt.Errorf("TODO(tandrii)")
	return []string{"test"}, nil
}

func Isolate(done <-chan struct{}, trees <-chan Tree) (<-chan map[string]string, <-chan FileAsset, <-chan error) {
	type result struct {
		target string
		hash   string
		err    error
	}
	chResults := make(chan result)
	chFileAssets := make(chan FileAsset)
	go func() {
		var wg sync.WaitGroup
		for tree := range trees {
			wg.Add(1)
			go func() {
				targetName := common.GetFileNameWithoutExtension(tree.Opts.Isolated)
				treeIsolatedHashes, err := isolateTree(done, tree, chFileAssets)
				chResults <- result{targetName, treeIsolatedHashes[0], err}
				wg.Done()
			}()
		}
		wg.Wait()
		close(chFileAssets)
		close(chResults)
	}()
	// Buffer these two channels, as we don't want blocking send, and they'll have at most 1 item.
	chIsolateHashes := make(chan map[string]string, 1)
	chError := make(chan error, 1)
	go func() {
		defer close(chError)
		defer close(chIsolateHashes)
		isolateHashes := map[string]string{}
		for r := range chResults {
			if r.err != nil {
				// TODO(tandrii): this used to be ignored in Py-swarming.
				chError <- r.err
				return
			}
			isolateHashes[r.target] = r.hash
		}
		chIsolateHashes <- isolateHashes
		chError <- nil // Indicate success.
	}()
	return chIsolateHashes, chFileAssets, chError
}

//prepareItemsForUpload filters out duplicated FileAsset and converts them to isolateserver.FileItem.
func prepareItemsForUpload(done <-chan struct{}, chIn <-chan FileAsset) <-chan isolateserver.UploadItem {
	chOut := make(chan isolateserver.UploadItem)
	go func() {
		defer close(chOut)
		seen := map[string]bool{}
		skipped := 0
		for fa := range chIn {
			if !fa.IsSymlink() && !seen[fa.fullPath] {
				seen[fa.fullPath] = true
				select {
				case chOut <- fa.ToUploadItem():
				case <-done:
					return
				}
			} else {
				skipped++
			}
		}
		log.Printf("Skipped %d duplicated entries", skipped)
	}()
	return chOut
}

func Archive(done <-chan struct{}, chFileAssets <-chan FileAsset, namespace string, server string) <-chan error {
	chError := make(chan error, 1)
	go func() {
		defer close(chError)
		s := isolateserver.NewStorage(server, namespace)
		if err := s.Connect(); err != nil {
			chError <- err
			return
		}
		chFilesToUpload := prepareItemsForUpload(done, chFileAssets)
		chError <- s.Upload(done, chFilesToUpload)
	}()
	return chError
}
