// Copyright © 2026 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !reference
// +build !reference

package reference

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

func TestShouldFilter(t *testing.T) {
	// Test context.Context should be filtered
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	assert.True(t, shouldFilter(contextType), "context.Context should be filtered")

	// Test error should be filtered
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	assert.True(t, shouldFilter(errorType), "error should be filtered")

	// Test string should not be filtered
	stringType := reflect.TypeOf("")
	assert.False(t, shouldFilter(stringType), "string should not be filtered")

	// Test int should not be filtered
	intType := reflect.TypeOf(0)
	assert.False(t, shouldFilter(intType), "int should not be filtered")
}

func TestGetType(t *testing.T) {
	// Test with concrete type
	str := "test"
	assert.Equal(t, reflect.TypeOf(str), getType(str))

	// Test with pointer type
	strPtr := &str
	assert.Equal(t, reflect.TypeOf(str), getType(strPtr))

	// Test with struct
	type testStruct struct {
		Name string
	}
	ts := testStruct{Name: "test"}
	assert.Equal(t, reflect.TypeOf(ts), getType(ts))

	// Test with pointer to struct
	assert.Equal(t, reflect.TypeOf(ts), getType(&ts))

	// Test with int
	num := 42
	assert.Equal(t, reflect.TypeOf(num), getType(num))
}

func TestGetRelativePath(t *testing.T) {
	// Test depth 0
	path0 := getRelativePath(0)
	assert.Contains(t, path0, "doc-site")
	assert.Contains(t, path0, "docs")
	assert.Contains(t, path0, "reference")
	assert.NotContains(t, path0, "..")

	// Test depth 1
	path1 := getRelativePath(1)
	assert.Contains(t, path1, "..")
	assert.Contains(t, path1, "doc-site")

	// Test depth 2
	path2 := getRelativePath(2)
	countDots := 0
	for i := 0; i < len(path2)-1; i++ {
		if path2[i:i+2] == ".." {
			countDots++
		}
	}
	assert.Equal(t, 2, countDots)

	// Test depth 5
	path5 := getRelativePath(5)
	countDots5 := 0
	for i := 0; i < len(path5)-1; i++ {
		if path5[i:i+2] == ".." {
			countDots5++
		}
	}
	assert.Equal(t, 5, countDots5)
}

func TestIsEnum(t *testing.T) {
	// Test with regular type (not enum)
	stringType := reflect.TypeOf("")
	assert.False(t, isEnum(stringType), "string is not an enum")

	// Test with int
	intType := reflect.TypeOf(0)
	assert.False(t, isEnum(intType), "int is not an enum")

	// Test with struct
	type testStruct struct {
		Name string
	}
	structType := reflect.TypeOf(testStruct{})
	assert.False(t, isEnum(structType), "struct is not an enum")
}

func TestGetTypeWithInterface(t *testing.T) {
	// Test with interface that wraps a concrete type
	var i interface{} = "test"
	resultType := getType(i)
	assert.Equal(t, reflect.TypeOf(""), resultType)

	// Test with interface that wraps an int
	var intInterface interface{} = 42
	intResultType := getType(intInterface)
	assert.Equal(t, reflect.TypeOf(0), intResultType)
}

func TestGetTypeWithStructPointer(t *testing.T) {
	type testStruct struct {
		Value int
		Name  string
	}

	s := &testStruct{Value: 100, Name: "test"}
	result := getType(s)

	// Should return the type of the struct, not the pointer
	assert.Equal(t, "testStruct", result.Name())
	assert.Equal(t, reflect.Struct, result.Kind())
}

func TestGetIncludeFileNotFound(t *testing.T) {
	ctx := i18n.WithLang(context.Background(), language.AmericanEnglish)
	// Use a path that won't have the _includes directory
	result, err := getIncludeFile(ctx, "/nonexistent/path", "test_type")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetIncludeFileSuccess(t *testing.T) {
	ctx := i18n.WithLang(context.Background(), language.AmericanEnglish)

	// Create a temporary directory structure
	tmpDir := t.TempDir()
	includesDir := filepath.Join(tmpDir, "_includes")
	err := os.MkdirAll(includesDir, 0755)
	assert.NoError(t, err)

	// Create a test description file
	descFile := filepath.Join(includesDir, "test_type_description.md")
	testContent := "# Test Type Description\n\nThis is a test description."
	err = os.WriteFile(descFile, []byte(testContent), 0644)
	assert.NoError(t, err)

	// Test the function
	result, err := getIncludeFile(ctx, tmpDir, "test_type")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, string(result), "include-markdown")
	assert.Contains(t, string(result), "test_type_description.md")
}

func TestGetIncludeFilePathAbsError(t *testing.T) {
	ctx := i18n.WithLang(context.Background(), language.AmericanEnglish)
	// This is tricky to test as filepath.Abs rarely fails, but we can test the error path
	// by using a normal path that doesn't exist
	result, err := getIncludeFile(ctx, "/tmp/nonexistent_dir_12345", "type")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestShouldFilterWithDifferentTypes(t *testing.T) {
	// Test with various types
	tests := []struct {
		name     string
		typeFunc func() interface{}
		expect   bool
	}{
		{
			name:     "string",
			typeFunc: func() interface{} { return "" },
			expect:   false,
		},
		{
			name:     "int",
			typeFunc: func() interface{} { return 0 },
			expect:   false,
		},
		{
			name:     "bool",
			typeFunc: func() interface{} { return false },
			expect:   false,
		},
		{
			name:     "slice",
			typeFunc: func() interface{} { return []string{} },
			expect:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := tt.typeFunc()
			typ := getType(val)
			result := shouldFilter(typ)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestGetRelativePathDepthVariations(t *testing.T) {
	tests := []struct {
		depth         int
		shouldContain string
	}{
		{0, "reference"},
		{1, ".."},
		{2, ".."},
		{3, ".."},
		{10, ".."},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.depth)+'0'), func(t *testing.T) {
			path := getRelativePath(tt.depth)
			assert.Contains(t, path, tt.shouldContain)
			assert.Contains(t, path, "reference")
		})
	}
}

func TestIsEnumWithDifferentTypes(t *testing.T) {
	tests := []struct {
		name    string
		typeVal interface{}
		expect  bool
	}{
		{"string", "", false},
		{"int", 0, false},
		{"float64", 0.0, false},
		{"bool", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := getType(tt.typeVal)
			result := isEnum(typ)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestGetTypePointerToPointer(t *testing.T) {
	// Test with a value
	val := "test"
	ptr := &val
	ptrPtr := &ptr

	result := getType(ptrPtr)
	// Should get the type of the first pointer dereference
	assert.Equal(t, "*string", result.String())
}

func TestGetTypeWithSlice(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := getType(slice)
	assert.Equal(t, "string", result.Elem().Name())
}
