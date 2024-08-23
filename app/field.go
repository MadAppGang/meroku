package main

type FieldType string

const (
	InputString FieldType = "INPUT_STRING"
	InputInt    FieldType = "INPUT_INT"
	SelectOne   FieldType = "SELECT_ONE"
	SelectMany  FieldType = "SELECT_MANY"
	Checkbox    FieldType = "CHECKBOX"
	Path        FieldType = "PATH"
	Text        FieldType = "TEXT"
)
