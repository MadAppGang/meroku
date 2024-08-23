package main

import (
	"strings"
	"unicode"
)

func insertAt[T any](slice []T, element T, index int) []T {
	if index < 0 || index > len(slice) {
		panic("Index out of range")
	}

	// Create a new slice with capacity for one more element
	result := make([]T, len(slice)+1)

	// Copy elements before the insertion point
	copy(result[:index], slice[:index])

	// Insert the new element
	result[index] = element

	// Copy elements after the insertion point
	copy(result[index+1:], slice[index:])

	return result
}

func replaceAt[T any](slice []T, element T, index int) []T {
	if index < 0 || index >= len(slice) {
		panic("Index out of range")
	}

	// Create a new slice with the same length as the original
	result := make([]T, len(slice))

	// Copy all elements from the original slice
	copy(result, slice)

	// Replace the element at the specified index
	result[index] = element

	return result
}

func splitStringByWidth(s string, maxWidth int) []string {
	var result []string
	runes := []rune(s)

	for len(runes) > 0 {
		if len(runes) <= maxWidth {
			result = append(result, string(runes))
			break
		}

		result = append(result, string(runes[:maxWidth]))
		runes = runes[maxWidth:]
	}

	return result
}

func splitStringByWidthStripLeftWS(s string, maxWidth int) []string {
	var result []string
	runes := []rune(s)

	for len(runes) > 0 {
		if len(runes) <= maxWidth {
			result = append(result, strings.Trim(string(runes), " "))
			break
		}

		result = append(result, strings.Trim(string(runes[:maxWidth]), " "))
		runes = runes[maxWidth:]
	}

	return result
}

func wrapText(text string, maxWidth int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var result strings.Builder
	lineLength := 0

	for i, word := range words {
		wordLength := len(word)

		if lineLength+wordLength > maxWidth {
			if lineLength > 0 {
				result.WriteString("\n")
			}
			result.WriteString(word)
			lineLength = wordLength
		} else {
			if lineLength > 0 {
				result.WriteString(" ")
				lineLength++
			}
			result.WriteString(word)
			lineLength += wordLength
		}

		// Handle punctuation at the end of a line
		if i < len(words)-1 && lineLength == maxWidth && unicode.IsPunct(rune(words[i+1][0])) {
			result.WriteString("\n")
			lineLength = 0
		}
	}

	return result.String()
}
