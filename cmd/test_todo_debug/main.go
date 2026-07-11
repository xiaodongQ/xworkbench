package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/xiaodongQ/xworkbench/internal/todo"
)

func main() {
	dir, _ := os.MkdirTemp("", "test_todo_*")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "todo.md")
	os.WriteFile(path, []byte("# Todo\n"), 0644)

	fmt.Println("Step 1: AddAndWrite Parent 1")
	lineNo, err := todo.AddAndWrite(path, "Parent 1", "", nil, "")
	fmt.Printf("  lineNo=%d err=%v\n", lineNo, err)
	data, _ := os.ReadFile(path)
	fmt.Printf("  File content: %q\n", string(data))
	lines := strings.Split(string(data), "\n")
	fmt.Printf("  len(lines)=%d\n", len(lines))
	
	fmt.Println("\nStep 2: AddChildAndWrite Child 1.1 to line 2")
	err = todo.AddChildAndWrite(path, 2, "Child 1.1", "", false)
	fmt.Printf("  err=%v\n", err)
	data, _ = os.ReadFile(path)
	fmt.Printf("  File content: %q\n", string(data))
	lines = strings.Split(string(data), "\n")
	fmt.Printf("  len(lines)=%d\n", len(lines))
	
	fmt.Println("\nStep 3: AddChildAndWrite Child 1.2 to line 2")
	err = todo.AddChildAndWrite(path, 2, "Child 1.2", "", false)
	fmt.Printf("  err=%v\n", err)
	data, _ = os.ReadFile(path)
	fmt.Printf("  File content: %q\n", string(data))
	lines = strings.Split(string(data), "\n")
	fmt.Printf("  len(lines)=%d\n", len(lines))
	
	fmt.Println("\nStep 4: AddAndWrite Parent 2")
	lineNo, err = todo.AddAndWrite(path, "Parent 2", "", nil, "")
	fmt.Printf("  lineNo=%d err=%v\n", lineNo, err)
	data, _ = os.ReadFile(path)
	fmt.Printf("  File content: %q\n", string(data))
}
