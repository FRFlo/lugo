package main

import "testing"

func TestNativeParamsKeepsCharPointers(t *testing.T) {
	params, docParams := nativeParams(NativeDefinition{
		Params: []NativeParam{
			{Name: "commandString", Type: "char*"},
			{Name: "description", Type: "char*"},
			{Name: "defaultMapper", Type: "const char*"},
			{Name: "defaultParameter", Type: "char *"},
			{Name: "outValue", Type: "int*"},
		},
	})

	wantParams := "commandString, description, defaultMapper, defaultParameter"
	if params != wantParams {
		t.Fatalf("params = %q, want %q", params, wantParams)
	}

	wantDocParams := "---@param commandString string\n" +
		"---@param description string\n" +
		"---@param defaultMapper string\n" +
		"---@param defaultParameter string\n"
	if docParams != wantDocParams {
		t.Fatalf("docParams = %q, want %q", docParams, wantDocParams)
	}
}
