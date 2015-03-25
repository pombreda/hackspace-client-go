// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
)

import . "chromium.googlesource.com/infra/swarming/client-go/internal/types"

type ConfigValueOfKey struct {
	Value   string
	IsBound bool
}

func (c *ConfigValueOfKey) Set(value string) {
	c.Value = value
	c.IsBound = true
}

type ConfigName []ConfigValueOfKey

func compareConfigNames(lhs, rhs ConfigName) int {
	// Bound value is less than unbound one.
	assert(len(lhs) == len(rhs))
	for k, l := range lhs {
		r := rhs[k]
		if l.IsBound && r.IsBound {
			if l.Value < r.Value {
				return -1
			} else if l.Value > r.Value {
				return 1
			}
		} else if l.IsBound {
			return -1
		} else if r.IsBound {
			return 1
		}
	}
	return 0
}

func (c *ConfigName) Equals(o ConfigName) bool {
	return reflect.DeepEqual(c, o)
}

type ConfigPair struct {
	Key   ConfigName
	Value ConfigSettings
}

type ConfigPairs []ConfigPair

func (c ConfigPairs) Len() int {
	return len(c)
}

func (c ConfigPairs) Less(i, j int) bool {
	// Compare only bounded values of .key
	lhs, rhs := c[i].Key, c[j].Key
	return compareConfigNames(lhs, rhs) < 0
}

func (c ConfigPairs) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// Config Represents a processed .isolate file.
//
//  Stores the file in a processed way, split by configuration.
//
//  At this point, we don't know all the possibilities. So mount a partial view
//  that we have.
//
//  This class doesn't hold isolate_dir, since it is dependent on the final
//  configuration selected. It is implicitly dependent on which .isolate defines
//  the 'command' that will take effect.
type Configs struct {
	FileComment []byte
	// Contains names only; the order is same as in ByConfig.
	ConfigVariables []string
	// Since Go doesn't allow slices as map keys, this is stored a list of (config_key, config_value).
	// The config key are lists of values of vars in the same order as ConfigSettings.
	ByConfig []ConfigPair
	// TODO(tandrii): maybe re-invent the hashing wheel here fo faster lookups by key?
}

func (c *Configs) Init(fileComment []byte, configVariables []string) {
	c.FileComment = fileComment
	assert(sort.IsSorted(sort.StringSlice(c.ConfigVariables)))
	c.ConfigVariables = configVariables
	c.ByConfig = []ConfigPair{}
}

// GetConfig returns all configs that matches this config as a single ConfigSettings.
//
// Returns an empty ConfigSettings if none apply.
func (c *Configs) GetConfig(configName ConfigName) (ConfigSettings, error) {
	out := ConfigSettings{}
	// Order ByConfig according to compareConfigNames function.
	sort.Sort(ConfigPairs(c.ByConfig))
	for _, pair := range c.ByConfig {
		ok := true
		for i, confValueOfKey := range configName {
			if pair.Key[i].IsBound && pair.Key[i] != confValueOfKey {
				ok = false
				break
			}
		}
		if ok {
			var err error
			if out, err = out.Union(pair.Value); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// GetExactConfig returns ConfigSettings for exactly given configName key.
// This func exists because ByConfig is not a map.
func (c *Configs) GetExactConfig(key ConfigName) (ConfigSettings, bool) {
	for _, kv := range c.ByConfig {
		if kv.Key.Equals(key) {
			return kv.Value, true
		}
	}
	return ConfigSettings{}, false
}

// SetConfig sets the ConfigSettings for this key.
//
// The key is a tuple of bounded or unbounded variables. The global variable
// is the key where all values are unbounded.
func (c *Configs) SetConfig(key ConfigName, value ConfigSettings) {
	assert(len(key) == len(c.ConfigVariables))
	_, exists := c.GetExactConfig(key)
	assert(!exists)
	c.ByConfig = append(c.ByConfig, ConfigPair{key, value})
}

// Union returns a new Configs instance, the union of variables from self and rhs.
//
// Uses lhs.file_comment if available, otherwise rhs.file_comment.
// It keeps config_variables sorted in the output.
func (lhs *Configs) Union(rhs Configs) (Configs, error) {
	// Merge the keys of config_variables for each Configs instances. All the new
	// variables will become unbounded. This requires realigning the keys.
	out := Configs{}
	if len(lhs.FileComment) == 0 {
		out.Init(rhs.FileComment, lhs.getConfigVarsUnion(rhs))
	} else {
		out.Init(lhs.FileComment, lhs.getConfigVarsUnion(rhs))
	}

	ByConfig := ConfigPairs(append(
		lhs.expandConfigVariables(out.ConfigVariables),
		rhs.expandConfigVariables(out.ConfigVariables)...))
	if len(ByConfig) == 0 {
		return out, nil
	}
	// Take union of ConfigSettings with the same ConfigName (key).
	sort.Sort(ByConfig)
	last := ByConfig[0]
	for _, curr := range ByConfig[1:] {
		if compareConfigNames(last.Key, curr.Key) == 0 {
			var err error
			if last.Value, err = last.Value.Union(curr.Value); err != nil {
				return out, err
			}
		} else {
			out.SetConfig(last.Key, last.Value)
			last = curr
		}
	}
	out.SetConfig(last.Key, last.Value)
	return out, nil
}

// getConfigVarsUnion returns a sorted set of union of ConfigVariables of two Configs.
func (lhs *Configs) getConfigVarsUnion(rhs Configs) []string {
	varSet := append([]string{}, lhs.ConfigVariables...)
	varSet = append(varSet, rhs.ConfigVariables...)
	if len(varSet) == 0 {
		return varSet
	}
	sort.Strings(varSet)
	unique := varSet[:1]
	last := unique[0]
	for _, cur := range varSet[1:] {
		if cur == last {
			continue
		}
		unique = append(unique, cur)
		last = cur
	}
	return unique
}

// expandConfigVariables returns new ByConfig value.
func (c *Configs) expandConfigVariables(newConfigVars []string) []ConfigPair {
	// Get mapping from old config vars list to new one.
	mapping := make([]int, len(newConfigVars))
	i := 0
	for n, nk := range newConfigVars {
		if i == len(c.ConfigVariables) || c.ConfigVariables[i] > nk {
			mapping[n] = -1
		} else if c.ConfigVariables[i] == nk {
			mapping[n] = i
			i++
		} else {
			// Must never happens because newConfigVars and c.configVariables are sorted ASC,
			// and newConfigVars contain c.configVariables as a subset.
			assert(c.ConfigVariables[i] >= nk)
		}
	}
	// Expands ConfigName to match newConfigVars.
	getNewConfigName := func(old ConfigName) ConfigName {
		var out ConfigName = make([]ConfigValueOfKey, len(mapping))
		for i, m := range mapping {
			if m != -1 {
				out[i] = old[m]
			}
		}
		return out
	}
	// Compute new ByConfig.
	out := make([]ConfigPair, len(c.ByConfig))
	for i, kv := range c.ByConfig {
		out[i] = ConfigPair{getNewConfigName(kv.Key), kv.Value}
	}
	return out
}

// ConfigSettings Represents the dependency variables for a single build configuration.
//
//  The structure is immutable.
//
//  .command and .isolate_dir describe how to run the command. .isolate_dir uses
//      the OS' native path separator. It must be an absolute path, it's the path
//      where to start the command from.
//  .files is the list of dependencies. The items use '/' as a path separator.
//  .read_only describe how to map the files.
type ConfigSettings struct {
	Files      []string
	Command    []string
	ReadOnly   int
	IsolateDir string
}

func (c *ConfigSettings) Init(values ParsedIsolate, isolateDir string) {
	if isolateDir == "" {
		// It must be an empty object if isolate_dir is not set.
		assert(values.IsEmpty(), values)
	} else {
		assert(filepath.IsAbs(isolateDir))
	}
	c.IsolateDir = isolateDir
	c.Files = values.Files
	sort.Strings(c.Files)
	c.Command = values.Command
	c.ReadOnly = values.ReadOnly
}

// Union merges two config settings together into a new instance.
//
// A new instance is not created and self or rhs is returned if the other
// object is the empty object.
//
// self has priority over rhs for .command. Use the same .isolate_dir as the
// one having a .command.
//
// Dependencies listed in rhs are patch adjusted ONLY if they don't start with
// a path variable, e.g. the characters '<('.
func (lhs *ConfigSettings) Union(rhs ConfigSettings) (ConfigSettings, error) {
	// When an object has .isolate_dir == "", it means it is the empty object.
	if lhs.IsolateDir == "" {
		return rhs, nil
	}
	if rhs.IsolateDir == "" {
		return *lhs, nil
	}

	if common.IsWin32() {
		assert(strings.ToLower(lhs.IsolateDir) == strings.ToLower(rhs.IsolateDir))
	}

	// Takes the difference between the two isolate_dir. Note that while
	// isolate_dir is in native path case, all other references are in posix.
	var useRhs bool
	var command []string
	if len(lhs.Command) > 0 {
		useRhs = false
		command = lhs.Command
	} else if len(rhs.Command) > 0 {
		useRhs = true
		command = rhs.Command
	} else {
		// If self doesn't define any file, use rhs.
		useRhs = len(lhs.Files) == 0
	}

	readOnly := rhs.ReadOnly
	if lhs.ReadOnly != -1 {
		readOnly = lhs.ReadOnly
	}

	lRelCwd, rRelCwd := lhs.IsolateDir, rhs.IsolateDir
	lFiles, rFiles := lhs.Files, rhs.Files
	if useRhs {
		// Rebase files in rhs.
		lRelCwd, rRelCwd = rhs.IsolateDir, lhs.IsolateDir
		lFiles, rFiles = rhs.Files, lhs.Files
	}

	var result ConfigSettings

	rebasePath, err := filepath.Rel(rRelCwd, lRelCwd)
	if err != nil {
		return result, err
	}
	rebasePath = strings.Replace(rebasePath, string(os.PathSeparator), "/", 0)

	files := make([]string, len(lFiles)+len(rFiles))
	copy(files, lFiles)
	for i, f := range rFiles {
		// Rebase item.
		if !(strings.HasPrefix(f, "<(") || rebasePath == ".") {
			f = common.PosixpathJoin(rebasePath, f)
		}
		files[i+len(lFiles)] = f
	}
	sort.Strings(files)
	result.Init(ParsedIsolate{command, files, readOnly}, lRelCwd)
	return result, nil
}

type UntypedIsolate struct {
	dict interface{}
}

func (p *UntypedIsolate) IsEmpty() bool {
	if p.dict == nil {
		return true
	}
	if m, ok := p.dict.(map[string]interface{}); ok {
		return len(m) == 0
	}
	return false
}

type ParsedIsolate struct {
	Command []string `json:"command"`
	Files   []string `json:"files"`
	// TODO(tandrii): read_only has 1 as default, according to specs.
	// Python-isolate also uses None as undefined, this code uses -1.
	// Golang's default is currently just 0.
	ReadOnly int `json:"read_only"`
}

func (p *ParsedIsolate) IsEmpty() bool {
	return len(p.Command) == 0 && len(p.Files) == 0
}

// LoadIsolateAsConfig parses one .isolate file and returns a Configs instance.
//
//  Arguments:
//    isolate_dir: only used to load relative includes so it doesn't depend on
//                 cwd.
//    value: is the loaded dictionary that was defined in the gyp file.
//    file_comment: comments found at the top of the file so it can be preserved.
//
//  The expected format is strict, anything diverting from the format below will result in error:
//  {
//    'includes': [
//      'foo.isolate',
//    ],
//    'conditions': [
//      ['OS=="vms" and foo=42', {
//        'variables': {
//          'command': [
//            ...
//          ],
//          'files': [
//            ...
//          ],
//          'read_only': 0,
//        },
//      }],
//      ...
//    ],
//    'variables': {
//      ...
//    },
//  }
func LoadIsolateAsConfig(isolateDir string, content []byte, fileComment []byte) (Configs, error) {
	var isolate Configs
	// TODO: can we parse this without Python?
	err := common.ExecPyHelperTyped(&isolate, "load_isolate_as_config", content, isolateDir)
	isolate.FileComment = fileComment
	return isolate, err
}

// LoadIsolateForConfig loads the .isolate file and returns the information unprocessed but
// filtered for the specific OS.
//
// Returns:
// tuple of command, dependencies, read_only flag, isolate_dir.
// The dependencies are fixed to use os.path.sep.
func LoadIsolateForConfig(isolateDir string, content []byte, configVariables KeyVars) (
	[]string, []string, int, string, error) {
	// Load the .isolate file, process its conditions, retrieve the command and dependencies.
	isolate, err := LoadIsolateAsConfig(isolateDir, content, nil)
	if err != nil {
		return nil, nil, -1, "", err
	}
	configName, err := computeConfigName(isolate, configVariables)
	if err != nil {
		return nil, nil, -1, "", err
	}
	// A configuration is to be created with all the combinations of free variables.
	config, err := isolate.GetConfig(configName)
	if err != nil {
		return nil, nil, -1, "", err
	}
	dependencies := make([]string, len(config.Files))
	for i, f := range config.Files {
		dependencies[i] = strings.Replace(f, "/", string(os.PathSeparator), -1)
	}
	return config.Command, dependencies, config.ReadOnly, config.IsolateDir, nil
}

func computeConfigName(isolate Configs, configVariables KeyVars) (ConfigName, error) {
	out := []ConfigValueOfKey{}
	missingVars := []string{}
	for _, variable := range isolate.ConfigVariables {
		if value, ok := configVariables[variable]; ok {
			out = append(out, ConfigValueOfKey{value, true})
		} else {
			missingVars = append(missingVars, variable)
		}
	}
	if len(missingVars) > 0 {
		sort.Strings(missingVars)
		return ConfigName{}, fmt.Errorf("These configuration variables were missing from the command line: %v",
			missingVars)
	}
	return ConfigName(out), nil
}
