package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

var (
	inputFile  string
	outputFile string
)

func init() {
	flag.StringVar(&inputFile, "i", "", "input file")
	flag.StringVar(&outputFile, "o", "", "output file")
}

func main() {
	flag.Parse()
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [context_key:context_file ...]\n", os.Args[0])
		flag.PrintDefaults()
	}

	fmt.Println("hello world in=", inputFile, "out", outputFile)

	if err := ioutil.WriteFile(outputFile, []byte("hello\n"), 0644); err != nil {
		log.Fatal(err)
	}
}
