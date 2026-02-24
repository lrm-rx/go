package random

import "math/rand"

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStr(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func RandStrByCode(ls string, n int) string {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = ls[rand.Intn(len(ls))]
	}
	return string(b)
}
