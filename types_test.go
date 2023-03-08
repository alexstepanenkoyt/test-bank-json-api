package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAccount(t *testing.T) {
	acc, err := NewAccount("aa", "bb", "qwerty")
	assert.Nil(t, err)

	fmt.Printf("%+v\n", acc)
}
