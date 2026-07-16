/*
Copyright 2022 The kube-fledged authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"os"
	"strings"
	"testing"
)

// Sed basically does 'sed'-like string replacement on files and returns a string
func Sed(t *testing.T, old, new, filePath string) string {
	t.Helper()

	oldFileByteSlice, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file %s", filePath)
	}

	return strings.ReplaceAll(string(oldFileByteSlice), old, new)
}
