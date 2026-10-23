//go:build headless

package main

import "net/http"

const hasUI = false

func staticHandler() http.Handler { return http.NotFoundHandler() }
