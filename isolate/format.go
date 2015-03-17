// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadIsolateForConfig loads the .isolate file and returns the information unprocessed but
// filtered for the specific OS.
//
// Returns:
// tuple of command, dependencies, read_only flag, isolate_dir.
// The dependencies are fixed to use os.path.sep.
func LoadIsolateForConfig(isolateDir string, content []byte, configVariables KeyVars) (
	[]string, []string, bool, string, error) {
	// Load the .isolate file, process its conditions, retrieve the command and dependencies.
	isolate, err := LoadIsolateAsConfig(isolateDir, content, nil)
	if err != nil {
		return nil, nil, false, "", err
	}
	configName, err := computeConfigName(isolate, configVariables)
	if err != nil {
		return nil, nil, false, "", err
	}
	config := isolate.GetConfig(configName)
	dependencies := make([]string, len(config.Files))
	for i, f := range config.Files {
		dependencies[i] = strings.Replace(f, "/", string(os.PathSeparator), -1)
	}
	return config.Command, dependencies, config.ReadOnly, config.IsolateDir, nil
}

func computeConfigName(isolate Configs, configVariables KeyVars) ([]string, error) {
	configName := []string{}
	missingVars := []string{}
	for _, variable := range isolate.ConfigVariables {
		if value, ok := configVariables[variable]; ok {
			configName = append(configName, value)
		} else {
			missingVars = append(missingVars, variable)
		}
	}
	if len(missingVars) > 0 {
		sort.Strings(missingVars)
		return []string{}, fmt.Errorf("These configuration variables were missing from the command line: %v",
			missingVars)
	}
	return configName, nil
}

type ConfigValue struct {
	value     string
	isUnbound bool // if true, value is irrelevant.
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
	FileComment     []byte
	ConfigVariables []string // contains names only; the order is same as in byConfig.
	ByConfig        []ConfigValue
}

func (c *Configs) Init(fileComment []byte, configVariables []string) {
	c.FileComment = fileComment
	c.ByConfig = configVariables
	assert(
}

// GetConfig Returns all configs that matches this config as a single ConfigSettings.
//
// Returns an empty ConfigSettings if none apply.
func (c *Configs) GetConfig(configName []string) ConfigSettings {
	// TODO(maruel): Fix ordering based on the bounded values. The keys are not
	// necessarily sorted in the way that makes sense, they are alphabetically
	// sorted. It is important because the left-most takes predescence.
	out := ConfigSettings{}
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
	ReadOnly   bool
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
func LoadIsolateAsConfig(isolateDir string, value []byte, fileComments []byte) (Configs, error) {
	assert(filepath.IsAbs(isolateDir), isolateDir)
}
