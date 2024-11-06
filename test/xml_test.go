package test

import (
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type StuctA struct {
	FieldA string `xml:"fieldA" `
	FiledB string `xml:"uid filedB,omitempty,prefix"`
}

func TestSomething(t *testing.T) {
	// assert equality

	x := StuctA{
		FieldA: "valueA",
		FiledB: "valueB",
	}

	b, err := xml.Marshal(&x)

	assert.Nil(t, err)

	fmt.Println(string(b))
}
