// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"fmt"
)

// hacky stuff for faster debugging.
func assertNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func assert(condition bool, info ...interface{}) {
	if condition {
		return
	}
	panic(fmt.Errorf("assertion failed: %s", info))
}
