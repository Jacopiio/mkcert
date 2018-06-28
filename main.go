// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command mkcert is a simple zero-config tool to make development certificates.
package main

import (
	"crypto"
	"crypto/x509"
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

func main() {
	log.SetFlags(0)
	var installFlag = flag.Bool("install", false, "install the local root CA in the system trust store")
	var uninstallFlag = flag.Bool("uninstall", false, "uninstall the local root CA from the system trust store")
	flag.Parse()
	if *installFlag && *uninstallFlag {
		log.Fatalln("ERROR: you can't set -install and -uninstall at the same time")
	}
	(&mkcert{
		installMode: *installFlag, uninstallMode: *uninstallFlag,
	}).Run(flag.Args())
}

const rootName = "rootCA.pem"
const keyName = "rootCA-key.pem"

type mkcert struct {
	installMode, uninstallMode bool

	CAROOT string
	caCert *x509.Certificate
	caKey  crypto.PrivateKey

	// The system cert pool is only loaded once. After installing the root, checks
	// will keep failing until the next execution. TODO: maybe execve?
	// https://github.com/golang/go/issues/24540 (thanks, myself)
	ignoreCheckFailure bool
}

func (m *mkcert) Run(args []string) {
	m.CAROOT = getCAROOT()
	if m.CAROOT == "" {
		log.Fatalln("ERROR: failed to find the default CA location, set one as the CAROOT env var")
	}
	fatalIfErr(os.MkdirAll(m.CAROOT, 0755), "failed to create the CAROOT")
	m.loadCA()

	if m.installMode {
		m.install()
		if len(args) == 0 {
			return
		}
	} else if m.uninstallMode {
		m.uninstall()
		return
	} else if !m.check() {
		log.Println("Warning: the local CA is not installed in the system trust store! ⚠️")
		log.Println("Run \"mkcert -install\" to avoid verification errors ‼️")
	}

	if len(args) == 0 {
		log.Printf(`
Usage:

	$ mkcert -install
	Install the local CA in the system trust store.

	$ mkcert example.org
	Generate "example.org.pem" and "example.org-key.pem".

	$ mkcert example.com myapp.dev localhost 127.0.0.1 ::1
	Generate "example.com+4.pem" and "example.com+4-key.pem".

	$ mkcert '*.example.com'
	Generate "_wildcard.example.com.pem" and "_wildcard.example.com-key.pem".

	$ mkcert -uninstall
	Unnstall the local CA (but do not delete it).

Change the CA certificate and key storage location by setting $CAROOT.
`)
		return
	}

	hostnameRegexp := regexp.MustCompile(`(?i)^(\*\.)?[0-9a-z_-]([0-9a-z._-]*[0-9a-z_-])?$`)
	for _, name := range args {
		if ip := net.ParseIP(name); ip != nil {
			continue
		}
		if hostnameRegexp.MatchString(name) {
			continue
		}
		log.Fatalf("ERROR: %q is not a valid hostname or IP", name)
	}

	m.makeCert(args)
}

func getCAROOT() string {
	if env := os.Getenv("CAROOT"); env != "" {
		return env
	}

	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("LocalAppData")
	case "darwin":
		dir = os.Getenv("HOME")
		if dir == "" {
			return ""
		}
		dir = filepath.Join(dir, "Library", "Application Support")
	default: // Unix
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			dir = os.Getenv("HOME")
			if dir == "" {
				return ""
			}
			dir = filepath.Join(dir, ".local", "share")
		}
	}
	return filepath.Join(dir, "mkcert")
}

func (m *mkcert) install() {
	if m.check() {
		return
	}

	m.installPlatform()
	m.ignoreCheckFailure = true

	if m.check() { // useless, see comment on ignoreCheckFailure
		log.Print("The local CA is now installed in the system trust store! ⚡️\n\n")
	} else {
		log.Fatal("Installing failed. Please report the issue with details about your environment at https://github.com/FiloSottile/mkcert/issues/new 👎\n\n")
	}
}

func (m *mkcert) uninstall() {
	m.uninstallPlatform()
	log.Print("The local CA is now uninstalled from the system trust store! 👋\n\n")
}

func (m *mkcert) check() bool {
	if m.ignoreCheckFailure {
		return true
	}

	_, err := m.caCert.Verify(x509.VerifyOptions{})
	return err == nil
}

func fatalIfErr(err error, msg string) {
	if err != nil {
		log.Fatalf("ERROR: %s: %s", msg, err)
	}
}
