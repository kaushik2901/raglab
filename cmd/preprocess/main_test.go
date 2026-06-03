package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTag_Empty(t *testing.T) {
	tag := resolveTag("", "pre")
	assert.Contains(t, tag, "pre-")
	assert.Len(t, tag, len("pre-")+15)
}

func TestResolveTag_Provided(t *testing.T) {
	tag := resolveTag("my-custom-tag", "pre")
	assert.Equal(t, "my-custom-tag", tag)
}

func TestResolveTag_DifferentPrefix(t *testing.T) {
	tag := resolveTag("", "idx")
	assert.Contains(t, tag, "idx-")
}
