package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Input provides user input to the agent.
type Input interface {
	ReadLine(prompt string) (string, error)
}

// StdinInput is a basic line reader from stdin.
type StdinInput struct {
	scanner *bufio.Scanner
}

func NewStdinInput() *StdinInput {
	return &StdinInput{scanner: bufio.NewScanner(os.Stdin)}
}

func (s *StdinInput) ReadLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if s.scanner.Scan() {
		return strings.TrimSpace(s.scanner.Text()), nil
	}
	if err := s.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}
