// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"io/ioutil"
	"os"
	"testing"
)

func benchmarkHashFile(size int64, b *testing.B) {
	data := make([]byte, size)
	filepath := "/dev/shm/ram_please"
	ioutil.WriteFile(filepath, data, 0644)
	defer os.Remove(filepath)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, e := HashFile(filepath, "sha-1"); e != nil {
			panic(e)
		}
	}
	b.StopTimer()
}

func BenchmarkHashFile512(b *testing.B) {
	benchmarkHashFile(512, b)
}
func BenchmarkHashFile1K(b *testing.B) {
	benchmarkHashFile(1024, b)
}
func BenchmarkHashFile32K(b *testing.B) {
	benchmarkHashFile(32*1024, b)
}
func BenchmarkHashFile64K(b *testing.B) {
	benchmarkHashFile(64*1024, b)
}
func BenchmarkHashFile512K(b *testing.B) {
	benchmarkHashFile(512*1024, b)
}
func BenchmarkHashFile1M(b *testing.B) {
	benchmarkHashFile(1024*1024, b)
}
func BenchmarkHashFile5M(b *testing.B) {
	benchmarkHashFile(5*1024*1024, b)
}
func BenchmarkHashFile10M(b *testing.B) {
	benchmarkHashFile(10*1024*1024, b)
}
func BenchmarkHashFile100M(b *testing.B) {
	// tandrii: on my workstation, this takes 0.24s on average.
	//	cPython takes about 0.18-0.21, so this is good enough for now.
	benchmarkHashFile(100*1024*1024, b)
}
