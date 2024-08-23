package main

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

var (
	_ inputValue = intValue{}
	_ inputValue = stringValue{}
	_ inputValue = boolValue{}
	_ inputValue = sliceValue{}
)

type InputValueType int

const (
	InputValueTypeString InputValueType = iota + 100
	InputValueTypeInt
	InputValueTypeSlice
	InputValueTypeBool
)

type inputValue interface {
	String() string
	Int() int
	Slice() []string
	Bool() bool
	Type() InputValueType
}

type intValue struct {
	value int
}

func (i intValue) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("value", i.value),
		slog.String("type", "int"),
	)
}

func (i intValue) String() string {
	return fmt.Sprintf("%d", i.value)
}

func (i intValue) Int() int {
	return i.value
}

func (i intValue) Slice() []string {
	return []string{i.String()}
}

func (i intValue) Bool() bool {
	return i.value != 0
}

func (i intValue) Type() InputValueType {
	return InputValueTypeInt
}

type stringValue struct {
	value string
}

func (s stringValue) String() string {
	return s.value
}

func (s stringValue) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("value", s.value),
		slog.String("type", "string"),
	)
}

func (s stringValue) Int() int {
	i, err := strconv.Atoi(s.value)
	if err != nil {
		return 0
	}
	return i
}

func (s stringValue) Slice() []string {
	return []string{s.String()}
}

func (s stringValue) Bool() bool {
	b, err := strconv.ParseBool(s.value)
	if err != nil {
		return false
	}
	return b
}

func (s stringValue) Type() InputValueType {
	return InputValueTypeString
}

type boolValue struct {
	value bool
}

func (b boolValue) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("value", b.value),
		slog.String("type", "bool"),
	)
}

func (b boolValue) String() string {
	return fmt.Sprintf("%t", b.value)
}

func (b boolValue) Int() int {
	if b.value {
		return 1
	}
	return 0
}

func (b boolValue) Slice() []string {
	return []string{b.String()}
}

func (b boolValue) Bool() bool {
	return b.value
}

func (b boolValue) Type() InputValueType {
	return InputValueTypeBool
}

type sliceValue struct {
	value []string
}

func (s sliceValue) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("value", s.String()),
		slog.String("type", "slice"),
	)
}

func (s sliceValue) String() string {
	return strings.Join(s.value, ",")
}

func (s sliceValue) Int() int {
	i, err := strconv.Atoi(s.String())
	if err != nil {
		return 0
	}
	return i
}

func (s sliceValue) Slice() []string {
	return s.value
}

func (s sliceValue) Bool() bool {
	b, err := strconv.ParseBool(s.String())
	if err != nil {
		return false
	}
	return b
}

func (s sliceValue) Type() InputValueType {
	return InputValueTypeSlice
}
