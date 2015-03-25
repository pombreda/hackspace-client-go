// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package types implements which are shared internally.
//
// It's a separate package from internal common,
// so that one can just import it directly where it's used like this:
//	 import (
//		. "chromium.googlesource.com/infra/swarming/client-go/internal/types"
//   )
// to avoid verbosity of types.SharedType.
package types

type IsolateHash string
type KeyVars map[string]string
