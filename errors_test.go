// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"errors"
	"io"
	"testing"
)

func TestWrapTruncatedPreservesErrorIdentity(t *testing.T) {
	for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF} {
		err := wrapTruncated(cause)
		if !errors.Is(err, ErrTruncated) {
			t.Fatalf("error=%v, want ErrTruncated", err)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("error=%v, want underlying %v", err, cause)
		}
	}
}

func TestWrapTruncatedLeavesOtherErrorsUnchanged(t *testing.T) {
	if got := wrapTruncated(ErrFormat); got != ErrFormat {
		t.Fatalf("error=%v, want ErrFormat", got)
	}
}
