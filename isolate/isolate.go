// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package isolate implements the code to process '.isolate' files.
package isolate

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/maruel/interrupt"

	"crypto/sha1"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"chromium.googlesource.com/infra/swarming/client-go/isolateserver"
)
import . "chromium.googlesource.com/infra/swarming/client-go/internal/types"

const ISOLATED_GEN_JSON_VERSION = 1
const VALID_VARIABLE = "[A-Za-z_][A-Za-z_0-9]*"
const DISK_FILE_CHUNK = 1024 * 1024

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

type FileMetadata map[string]string

func (m *FileMetadata) IsSymlink() bool {
	return (*m)["l"] != ""
}
func (m *FileMetadata) IsHighPriority() bool {
	return (*m)["priority"] == "0"
}
func (m *FileMetadata) GetDigest() string {
	return (*m)["h"]
}

func (m *FileMetadata) GetSize() (int64, error) {
	return strconv.ParseInt((*m)["s"], 10, 64)
}

type FileAsset struct {
	FileMetadata
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
			Digest:           fa.FileMetadata["h"],
			Size:             fa.GetSize(),
			HighPriority:     fa.IsHighPriority(),
			CompressionLevel: 6,
		},
		fa.fullPath,
	}
	return &f
}

type SavedState struct {
	// Value of sys.platform so that the file is rejected if loaded from a
	// different OS. While this should never happen in practice, users are ...
	// "creative".
	OS string `json:"OS"`
	// Algorithm used to generate the hash. The only supported value is at the
	// time of writting 'sha-1'.
	Algo string `json:"algo"`
	// List of included .isolated files. Used to support/remember 'slave'
	// .isolated files. Relative path to isolated_basedir.
	ChildIsolatedFiles []string `json:"child_isolated_files"`
	// Cache of the processed command. This value is saved because .isolated
	// files are never loaded by isolate.py so it's the only way to load the
	// command safely.
	Command []string `json:"command"`
	// GYP variables that are used to generate conditions. The most frequent
	// example is 'OS'.
	ConfigVariables map[string]interface{} `json:"ConfigVariables"`
	// GYP variables that will be replaced in 'command' and paths but will not be
	// considered a relative directory.
	ExtraVariables map[string]string `json:"ExtraVariables"`
	// Cache of the files found so the next run can skip hash calculation.
	Files map[string]FileMetadata `json:"files"`
	// Path of the original .isolate file. Relative path to isolated_basedir.
	IsolateFile string `json:"isolate_file"`
	// GYP variables used to generate the .isolated files paths based on path
	// variables. Frequent examples are DEPTH and PRODUCT_DIR.
	PathVariables map[string]string `json:"PathVariables"`
	// If the generated directory tree should be read-only. Defaults to 1.
	ReadOnly bool `json:"read_only"`
	// Relative cwd to use to start the command.
	RelativeCwd string `json:"relative_cwd"`
	// Root directory the files are mapped from.
	RootDir string `json:"root_dir"`
	// Version of the saved state file format. Any breaking change must update
	// the value.
	Version string `json:"version"`

	cwd             string
	isolateFilepath string
	isolatedBasedir string
}
type CompleteState struct {
	SavedState
}

func (cs *CompleteState) LoadFromIsolated(isolated string) error {
	return nil
}
func (cs *CompleteState) InitializeDummy(cwd string) {
}
func (cs *CompleteState) InitIgnoreSavedState() {
}
func (cs *CompleteState) LoadFromIsolate(cwd, isolate string, opts ArchiveOptions) error {
	return nil
}
func (cs *CompleteState) FilesToMetadata() error {
	//TODO(tandrii): need sorting? For determinism?
	var err error
	for f, meta := range cs.SavedState.Files {
		filepath := path.Join(cs.RootDir, f)
		if cs.SavedState.Files[f], err = FileToMetadata(filepath, meta, cs.ReadOnly, cs.Algo); err != nil {
			return err
		}
	}
	return nil
}

func HashFile(filepath, algo string) (string, error) {
	if algo != "sha-1" {
		return "", fmt.Errorf("%s is not supported, only sha-1", algo)
	}
	h := sha1.New()
	f, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %s", filepath, err)
	}
	defer f.Close()
	// I(tandrii) benchmarked this with various buffer sizes of this copying.
	// Golang's Copy uses 32K, and raising this number to 1MB as it is in Python
	// didn't help in performance single threaded. But higher numbers are more
	// likely to suffer from multi-threaded cache polution. Lowering this number
	// doesn't seem to make a difference either.
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// FileToMetadata processes an input file, a dependency, and return meta data about it.
//
//  Behaviors:
//  - Retrieves the file mode, file size, file timestamp, file link
//    destination if it is a file link and calcultate the SHA-1 of the file's
//    content if the path points to a file and not a symlink.
//
//  Arguments:
//    filePath: File to act on.
//    prevdict: the previous dictionary. It is used to retrieve the cached sha-1
//              to skip recalculating the hash. Optional.
//    read_only: If 1 or 2, the file mode is manipulated. In practice, only save
//               one of 4 modes: 0755 (rwx), 0644 (rw), 0555 (rx), 0444 (r). On
//               windows, mode is not set since all files are 'executable' by
//               default.
//    algo:      Hashing algorithm used.
//
//  Returns:
//    The necessary dict to create a entry in the 'files' section of an .isolated
//    file.
func FileToMetadata(filePath string, prev FileMetadata, readOnly bool, algo string) (FileMetadata, error) {
	out := FileMetadata{}
	filestats, err := os.Lstat(filePath)
	if err != nil {
		return out, fmt.Errorf("file %s is missing", filePath)
	}
	is_link := (0 != filestats.Mode()&os.ModeSymlink)

	if !common.IsWindows() {
		// Ignore file mode on Windows since it's not really useful there.
		mask := os.ModePerm | os.ModeSticky | os.ModeSetgid | os.ModeSetuid
		filemode := int32(mask & filestats.Mode())
		// Remove write access for group and all access to 'others'.
		filemode &= ^(unix.S_IWGRP | unix.S_IRWXO)
		if readOnly {
			filemode &= ^(unix.S_IWUSR)
		}
		if filemode&(unix.S_IXUSR|unix.S_IRGRP) == (unix.S_IXUSR | unix.S_IRGRP) {
			// Only keep x group bit if both x user bit and group read bit are set.
			filemode |= unix.S_IXGRP
		} else {
			filemode &= ^unix.S_IXGRP
		}
		if is_link {
			out["m"] = string(filemode)
		}
	}

	// Used to skip recalculating the hash or link destination. Use the most recent# update time.
	// TODO(tandrii): is rounding tstamp important? Also, what unit is this?
	out["t"] = string(filestats.ModTime().Unix())
	if !is_link {
		// If the timestamp wasn't updated and the file size is still the same, carry on the sha-1.
		if prev["t"] == out["t"] && prev["s"] == out["s"] {
			// Reuse the previous hash if available.
			out["h"] = prev["h"]
		}
		if out["h"] == "" {
			if out["h"], err = HashFile(filePath, algo); err != nil {
				return out, err
			}
		}
	} else {
		// If the timestamp wasn't updated, carry on the link destination.
		if prev["t"] == out["t"] {
			// Reuse the previous link destination if available.
			out["l"] = prev["l"]
		}
		if out["l"] == "" {
			// The link could be in an incorrect path case. In practice, this only
			// happen on OSX on case insensitive HFS.
			// TODO(maruel): It'd be better if it was only done once, in
			// expand_directory_and_symlink(), so it would not be necessary to do again
			// here.
			symlinkValue, err := os.Readlink(filePath)
			if err != nil {
				return out, err
			}
			filedir, err := common.GetNativePathCase(path.Dir(filePath))
			if err != nil {
				return out, err
			}
			nativeDest, err := common.FixNativePathCase(filedir, symlinkValue)
			if err != nil {
				return out, err
			}
			out["l"], err = filepath.Rel(nativeDest, filedir)
			if err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func loadcompleteState(opts ArchiveOptions, cwd string, skipUpdate bool) (CompleteState, error) {
	// TODO(tandrii): is subdir handling required? I think not any more.
	// TODO(tandrii): assert absolute path of isolate or isolated.
	completeState := CompleteState{}
	if cwd_new, err := common.GetNativePathCase(cwd); err != nil {
		return completeState, err
	} else {
		cwd = cwd_new
	}
	if opts.Isolated != "" {
		// Load the previous state if it was present. Namely, "foo.isolated.state".
		// Note: this call doesn't load the .isolate file.
		if err := completeState.LoadFromIsolated(opts.Isolated); err != nil {
			return completeState, err
		}
	} else {
		if curCwd, err := os.Getwd(); err != nil {
			return completeState, err
		} else {
			// Constructs a dummy object that cannot be saved. Useful for temporary
			// commands like 'run'. There is no directory containing a .isolated file so
			// specify the current working directory as a valid directory.
			completeState.InitializeDummy(curCwd)
		}
	}
	isolate := ""
	if opts.Isolate == "" {
		if completeState.SavedState.IsolateFile == "" {
			if !skipUpdate {
				return completeState, errors.New("An .isolate file is required.")
			} else {
				isolate = ""
			}
		} else {
			isolate = completeState.SavedState.IsolateFile
		}
	} else {
		isolate = opts.Isolate
		if completeState.SavedState.IsolateFile != "" {
			if relIsolate, err := filepath.Rel(opts.Isolate,
				completeState.SavedState.isolatedBasedir); err != nil {
				return completeState, err
			} else if relIsolate != completeState.SavedState.IsolateFile {
				// This happens if the .isolate file was moved for example. In this case,
				// discard the saved state.
				log.Printf("warning: --isolated %s != %s as saved in %s. Discarding saved state",
					relIsolate, completeState.SavedState.IsolateFile,
					common.IsolatedFileToState(opts.Isolate))
				completeState = CompleteState{}
				completeState.InitIgnoreSavedState()
			}
		}
	}
	if !skipUpdate {
		if err := completeState.LoadFromIsolate(cwd, isolate, opts); err != nil {
			return completeState, err
		}
		if err := completeState.FilesToMetadata(); err != nil {
			return completeState, err
		}
	}
	return completeState, nil
}

func isolateTree(tree Tree, chFileAssets chan<- FileAsset) ([]IsolateHash, error) {
	//return nil, fmt.Errorf("TODO(tandrii)")
	return []IsolateHash{IsolateHash("test")}, nil
}

func Isolate(trees []Tree) (map[string]IsolateHash, []FileAsset, error) {
	chTrees := make(chan Tree, len(trees))
	for _, tree := range trees {
		chTrees <- tree
	}
	close(chTrees)
	chIsolateHashes, chFileAssets, chErrors := IsolateAsync(chTrees)
	var isolatedHashes map[string]IsolateHash
	var err error
	fileAssets := []FileAsset{}
	for err == nil {
		select {
		case i := <-chIsolateHashes:
			isolatedHashes = i
		case fa := <-chFileAssets:
			fileAssets = append(fileAssets, fa)
		case e := <-chErrors:
			err = e
		}
	}
	return isolatedHashes, fileAssets, err
}

func IsolateAsync(trees <-chan Tree) (<-chan map[string]IsolateHash, <-chan FileAsset, <-chan error) {
	type result struct {
		target string
		hash   IsolateHash
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
				treeIsolatedHashes, err := isolateTree(tree, chFileAssets)
				chResults <- result{targetName, treeIsolatedHashes[0], err}
				wg.Done()
			}()
		}
		wg.Wait()
		close(chFileAssets)
		close(chResults)
	}()
	// Buffer these two channels, as we don't want blocking send, and they'll have at most 1 item.
	chIsolateHashes := make(chan map[string]IsolateHash, 1)
	chError := make(chan error, 1)
	go func() {
		defer close(chError)
		defer close(chIsolateHashes)
		isolateHashes := map[string]IsolateHash{}
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
func prepareItemsForUpload(chIn <-chan FileAsset) <-chan isolateserver.UploadItem {
	chOut := make(chan isolateserver.UploadItem)
	go func() {
		defer close(chOut)
		seen := map[string]bool{}
		skipped := 0
		defer log.Printf("Skipped %d duplicated entries", skipped)

		for fa := range chIn {
			if !fa.IsSymlink() && !seen[fa.fullPath] {
				seen[fa.fullPath] = true
				select {
				case chOut <- fa.ToUploadItem():
				case <-interrupt.Channel:
					return
				}
			} else {
				skipped++
			}
		}
	}()
	return chOut
}

func Archive(fileAssets []FileAsset, namespace string, server string) error {
	chFileAssets := make(chan FileAsset, len(fileAssets))
	for _, fa := range fileAssets {
		chFileAssets <- fa
	}
	close(chFileAssets)
	chErrors := ArchiveAsync(chFileAssets, namespace, server)
	return <-chErrors
}

func ArchiveAsync(chFileAssets <-chan FileAsset, namespace string, server string) <-chan error {
	chError := make(chan error, 1)
	go func() {
		defer close(chError)
		s := isolateserver.NewStorage(server, namespace)
		if err := s.Connect(); err != nil {
			chError <- err
			return
		}
		chFilesToUpload := prepareItemsForUpload(chFileAssets)
		chError <- s.Upload(chFilesToUpload)
	}()
	return chError
}
