package main

import (
	"github.com/mgutz/ansi"
)

var choices = []string{
	"black",
	"red",
	"green",
	"yellow",
	"blue",
	"magenta",
	"cyan",
	"white",
}

var whichChoice = 0

func rainbow(s string) string {
	whichChoice++
	whichChoice = whichChoice % len(choices)
	return ansi.Color(s, choices[whichChoice])
}
