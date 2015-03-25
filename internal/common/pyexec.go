// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
)

func execCommand(program string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.Command(program, args...)
	cmd.Stderr = os.Stderr
	inwriter, _ := cmd.StdinPipe()
	inwriter.Write(input)
	inwriter.Close()
	out, err := cmd.Output()
	if err != nil {
		log.Printf("ERROR: %v with output %v\n", err, out)
		return out, err
	}
	return out, nil
}

func ExecPyHelper(run string, input []byte, args ...string) (interface{}, error) {
	var v interface{}
	err := ExecPyHelperTyped(&v, run, input, args...)
	return v, err
}

func ExecPyHelperTyped(v interface{}, run string, input []byte, args ...string) error {
	path := "../python_helper.py"
	fullArgs := append([]string{path, run}, args...)
	bytes, err := execCommand("python", input, fullArgs...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &v); err != nil {
		log.Printf("bad json [%v] from (%s)\n", err, bytes)
		return err
	}
	return nil
}
