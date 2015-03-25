// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import "testing"

func TestLoadIsolateAsConfig(t *testing.T) {
	_, err := LoadIsolateAsConfig("/s/swarming", []byte("{}"), []byte("# filecomment"))
	if err != nil {
		t.Error(err)
	}
}
