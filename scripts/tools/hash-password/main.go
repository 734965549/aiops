// hash-password 用 pkg/auth 默认 bcrypt cost(12) 计算密码哈希，供发布脚本调用。
package main

import (
	"fmt"
	"os"

	"github.com/734965549/aiops/pkg/auth"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hash-password <plaintext>")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(hash)
}
