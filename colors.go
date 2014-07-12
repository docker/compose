package main

import (
	"github.com/mgutz/ansi"
)

var choices = []string{
	"white",
	"magenta",
	"cyan",
	"red",
	"green",
	"yellow",
	"blue",
}

var whichChoice = 0

func rainbow(s string) string {
	whichChoice++
	whichChoice = whichChoice % len(choices)
	return ansi.Color(s, choices[whichChoice])
}
