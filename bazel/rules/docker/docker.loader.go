// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

type stringSliceFlag []string

func (f *stringSliceFlag) String() string {
	return strings.Join(*f, " ")
}

func (f *stringSliceFlag) Set(v string) error {
	if v != "" {
		*f = append(*f, v)
	}
	return nil
}

var (
	dockerTool  string
	dockerFlags stringSliceFlag
	imageTar    string
	imageTag    string
	output      string
	verbose     bool
)

func init() {
	flag.StringVar(&dockerTool, "docker-tool", "", "docker tool")
	flag.Var(&dockerFlags, "docker-flag", "docker flag (can be used more than once)")
	flag.StringVar(&imageTar, "image", "", "image tar file")
	flag.StringVar(&imageTag, "tag", "", "tag to apply to loaded image")
	flag.StringVar(&output, "o", "", "output file")
	flag.BoolVar(&verbose, "v", false, "verbose mode")
}

func main() {
	log.SetPrefix("go_docker_loader: ")
	flag.Parse()

	// Read image tar provided by container_pull.
	imageID, err := readImageID(imageTar)
	if err != nil {
		log.Fatal(err)
	}
	if verbose {
		log.Printf("From image tar %v got imageID=%v", imageTar, imageID)
	}

	// Check if the image already exists in the Docker daemon.
	tags, err := dockerInspectImageTags(imageID)
	if err != nil {
		// Load image.
		if err = dockerLoad(imageTar); err != nil {
			log.Fatal(err)
		}
	}
	if verbose {
		log.Printf("Image exists with tags=[%v]", strings.Join(tags, ", "))
	}

	// Check if the image is already tagged.
	needsTagged := true
	for _, tag := range tags {
		if tag == imageTag {
			needsTagged = false
			break
		}
	}

	// Tag the image.
	if needsTagged {
		if err = dockerTag(imageID, imageTag); err != nil {
			log.Fatal(err)
		}
		if verbose {
			log.Printf("imageID=%v tagged with %q", imageID, imageTag)
		}
	}

	// Write image coordinates for tests.
	f, err := os.Create(output)
	if err != nil {
		log.Fatalln("failed to create output file", err)
	}
	defer f.Close()

	if err = outputImageCoordinates(f, imageID, imageTag); err != nil {
		log.Fatal(err)
	}
	if verbose {
		log.Println("Output written to", output)
	}
}

func readImageID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	t := tar.NewReader(bufio.NewReader(f))
	for {
		hdr, err := t.Next()
		if err != nil {
			return "", fmt.Errorf("failed reading image tar: %w", err)
		}

		if hdr.Name != "manifest.json" {
			continue
		}

		buf := new(bytes.Buffer)
		if _, err = buf.ReadFrom(t); err != nil {
			return "", fmt.Errorf("failed to read manifest.json: %w", err)
		}

		return parseManifestJSON(buf.Bytes())
	}
}

func parseManifestJSON(data []byte) (string, error) {
	type manifest struct {
		Config string `json:"Config"`
	}

	var m []manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	// There should only be a single image in the tarball.
	if len(m) == 1 {
		return strings.TrimSuffix(m[0].Config, ".json"), nil
	}

	return "", fmt.Errorf("failed to parse manifest.json: expected a single "+
		"descriptor in the tarball manifest but found %d", len(m))
}

func dockerInspectImageTags(imageID string) ([]string, error) {
	cmd := exec.Command(dockerTool, dockerFlags...)
	cmd.Args = append(cmd.Args, "inspect", "--format={{json .RepoTags}}", imageID)
	if verbose {
		cmd.Stderr = os.Stderr
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var tags []string
	if err = json.Unmarshal(out, &tags); err != nil {
		return nil, fmt.Errorf("failed to parse docker inspect output of %q: %w", string(out), err)
	}

	return tags, nil
}

func dockerLoad(imageTar string) error {
	cmd := exec.Command(dockerTool, dockerFlags...)
	cmd.Args = append(cmd.Args, "load", "-i", imageTar)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load image %q: %w", imageTar, err)
	}
	return nil
}

func dockerTag(sourceImageID, tag string) error {
	cmd := exec.Command(dockerTool, dockerFlags...)
	cmd.Args = append(cmd.Args, "tag", sourceImageID, tag)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to tag image ID=%v with tag %q: %w", sourceImageID, tag, err)
	}
	return nil
}

func outputImageCoordinates(w io.Writer, imageID, tag string) error {
	// Write image coordinates for tests.
	type dockerImage struct {
		ImageID string `json:"image_id"`
		Tag     string `json:"tag"`
	}
	err := json.NewEncoder(w).Encode(dockerImage{
		ImageID: imageID,
		Tag:     imageTag,
	})
	if err != nil {
		return fmt.Errorf("failed to output image info: %w", err)
	}
	return nil
}
