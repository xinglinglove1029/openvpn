package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: genpass <plaintext>")
		os.Exit(1)
	}
	pw := os.Args[1]
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	if err != nil {
		fmt.Println("err:", err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}
