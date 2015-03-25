// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kr/pretty"
	"github.com/maruel/interrupt"
)
import . "chromium.googlesource.com/infra/swarming/client-go/internal/types"

// URLToHTTPS ensures the url is https://.
func URLToHTTPS(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" && u.Scheme != "https" {
		return "", errors.New("Only https:// scheme is accepted. It can be omitted.")
	}
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	if _, err = url.Parse(s); err != nil {
		return "", err
	}
	return s, nil
}

// IsDirectory returns true if path is a directory and is accessible.
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

// IsolatedFileToState returns a path to saved state file for an isolate file.
func IsolatedFileToState(isolate string) string {
	return isolate + ".state"
}

func GetNativePathCase(ipath string) (string, error) {
	// TODO(tandrii): memoize?
	if IsWindows() || IsMac() {
		return "", errors.New("TODO(tandrii): mac+win")
	} else {
		// Other platforms => likely Linux.
		opath := path.Clean(ipath)
		sep := string(os.PathSeparator)
		if strings.HasSuffix(ipath, sep) && strings.HasSuffix(opath, sep) {
			opath = opath + sep
		}
		// Probably useful in Go too.
		if opath == ipath {
			return ipath, nil
		} else {
			return opath, nil
		}
	}
}

func findItemNativeCase(root, item string) (string, error) {
	if IsWindows() {
		return "", errors.New("TODO(tandrii) findItemNativeCase for win")
	} else if IsMac() {
		return "", errors.New("TODO(tandrii) findItemNativeCase for mac")
	} else {
		if item == ".." {
			return item, nil
		}
		if root, err := GetNativePathCase(root); err != nil {
			return "", err
		} else if res, err := GetNativePathCase(path.Join(root, item)); err != nil {
			return "", err
		} else {
			return path.Base(res), nil
		}
	}
}

func FixNativePathCase(root, inpath string) (string, error) {
	nativeCasePath := root
	for _, rawPart := range strings.Split(inpath, string(os.PathSeparator)) {
		if rawPart == "" || rawPart == "." {
			break
		}
		if part, err := findItemNativeCase(nativeCasePath, rawPart); err != nil {
			return "", err
		} else {
			nativeCasePath = path.Join(nativeCasePath, part)
		}
	}
	return nativeCasePath, nil
}

func PosixpathJoin(a ...string) string {
	//TODO(tandrii): re-use a package for this?
	return strings.Replace(filepath.Join(a...), string(os.PathSeparator), "/", 0)
}

func GetFileNameWithoutExtension(path string) string {
	fname := filepath.Base(path)
	return strings.TrimSuffix(fname, filepath.Ext(fname))
}

func GetFileSize(path string) (int64, error) {
	if stat, err := os.Stat(path); err != nil {
		return 0, err
	} else {
		return stat.Size(), nil
	}
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func IsWin32() bool {
	return IsWindows() && runtime.GOARCH == "386"
}

func IsMac() bool {
	return runtime.GOOS == "darwin"
}

// StringsCollect accumulates string values from repeated flags.
// Use with flag.Var to accumlate values from "-flag s1 -flag s2".
type StringsCollect struct {
	Values *[]string
}

func (c *StringsCollect) String() string {
	return strings.Join(*c.Values, " ")
}

func (c *StringsCollect) Set(value string) error {
	*c.Values = append(*c.Values, value)
	return nil
}

// NKVArgCollect accumulates multiple key-value for a given flag.
// The only supported form is --flag key=value .
// If the same key appears several times, the value of last occurence is used.
type NKVArgCollect struct {
	Values  *KeyVars
	OptName string
}

func (c *NKVArgCollect) SetAsFlag(flags *flag.FlagSet, values *KeyVars,
	name string, usage string) {
	c.Values = values
	c.OptName = name
	flags.Var(c, name, usage)
}

func (c *NKVArgCollect) String() string {
	return pretty.Sprintf("%v", *c.Values)
}

func (c *NKVArgCollect) Set(value string) error {
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("please use %s FOO=BAR", c.OptName)
	}
	key, value := kv[0], kv[1]
	// TODO(tandrii): decode value as utf-8.
	(*c.Values)[key] = value
	return nil
}

// SendError sends error to error channel unless interrupted.
// Use this for timeley termination of gourotines.
func SendError(err error, chError chan<- error) {
	select {
	case chError <- err:
	case <-interrupt.Channel:
	}
}
