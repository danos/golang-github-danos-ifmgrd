// Copyright (c) 2019, AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015-2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

/* ifmgrd  is a daemon  that manages interface configuration.

Usage:

     -cpuprofile=<filename> Defines a file which to write a cpu
        profile that can be parsed with go pprof.  When defined, the
        daemon will begin recording cpu profile information when it
        receives a SIGUSR1 signal. Then on a subsequent SIGUSR1 it
        will write the profile information to the defined file.

	-socketfile=<filename> When defined configd will write its pid to
		the defined file (defualt: /run/ifmgrd/main.sock).

	-yangdir=<dir> Directory configd will load YANG files and watch
		for updates (default: /usr/share/configd/yang).

    -configdsocket=<filename> Specify the location of the configd socket
        with which we can proxy requests (default: /run/configd/main.sock).

	SIGUSR1 Issuing SIGUSR1 to the daemon will toggle run-time
		profiling. Profile data will be written to the file specified
		by the cpuprofile option.

*/
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"

	"github.com/coreos/go-systemd/activation"
	"github.com/danos/config/schema"
	"github.com/danos/config/yangconfig"
	"github.com/danos/ifmgrd"
	"github.com/danos/yang/compile"
)

var basepath string = "/run/ifmgrd"
var runningprof bool
var cpuproffile os.File

/* Command line options */
var cpuprofile string
var socket string
var yangdir string
var capabilities string
var configdsocket string

func sigstartprof() {
	sigch := make(chan os.Signal)
	signal.Notify(sigch, syscall.SIGUSR1)
	for {
		<-sigch
		if cpuprofile != "" {
			if !runningprof {
				cpuproffile, err := os.Create(cpuprofile)
				if err != nil {
					log.Fatal(err)
				}
				pprof.StartCPUProfile(cpuproffile)
				runningprof = true
			} else {
				pprof.StopCPUProfile()
				cpuproffile.Close()
				runningprof = false
			}
		}
	}
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	flag.StringVar(&cpuprofile, "cpuprofile",
		basepath+"/profile.pprof",
		"Write cpu profile to supplied file on SIGUSR1.")

	flag.StringVar(&socket, "socketfile",
		basepath+"/main.sock",
		"Path to socket used to comminicate with daemon.")

	flag.StringVar(&yangdir, "yangdir",
		"/usr/share/configd/yang",
		"Load YANG from specified directory.")

	flag.StringVar(&capabilities, "capabilities",
		compile.DefaultCapsLocation,
		"File specifying system capabilities")

	flag.StringVar(&configdsocket, "configdsocket",
		"/run/configd/main.sock",
		"Location where the configd socket resides")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

}

const tmppath = "/tmp/configd.org"
const newconfigdsocket = tmppath + "/main.sock"

func jugglemounts() error {
	//mkdir -p tmppath
	err := os.MkdirAll(tmppath, 0755)
	if err != nil {
		return err
	}

	//touch newsocket
	f, err := os.Create(newconfigdsocket)
	if err != nil {
		return err
	}
	f.Close()

	//mkdir -p basepath
	err = os.MkdirAll(basepath, 0755)
	if err != nil {
		return err
	}

	//mount --bind configdsocket newsocket
	err = syscall.Mount(configdsocket,
		newconfigdsocket, "", syscall.MS_BIND, "")
	if err != nil {
		return err
	}

	//mount --bind basepath $(dirname configdsocket)
	err = syscall.Mount(basepath,
		filepath.Dir(configdsocket), "", syscall.MS_BIND, "")
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var err error

	flag.Parse()

	fatal(jugglemounts())

	go sigstartprof()

	ycfg := yangconfig.NewConfig().IncludeYangDirs(yangdir).
		IncludeFeatures(capabilities).SystemConfig()

	st, err := schema.CompileDir(
		&compile.Config{
			YangLocations: ycfg.YangLocator(),
			Features:      ycfg.FeaturesChecker(),
			Filter:        compile.IsConfig},
		nil)
	fatal(err)

	ifmgrd.SchemaTree.Store(st)

	listeners, err := activation.Listeners(true)
	fatal(err)
	if len(listeners) == 0 {
		fmt.Println("No systemd listeners")
		if !os.IsNotExist(os.Remove(socket)) {
			fatal(err)
		}

		ua, err := net.ResolveUnixAddr("unix", socket)
		fatal(err)

		l, err := net.ListenUnix("unix", ua)
		fatal(err)

		err = os.Chmod(socket, 0770)
		fatal(err)

		listeners = append(listeners, l)
	}
	l := listeners[0]

	config := &ifmgrd.Config{
		Yangdir:       yangdir,
		Socket:        socket,
		Capabilities:  capabilities,
		ConfigdSocket: newconfigdsocket,
	}

	srv := ifmgrd.NewSrv(l.(*net.UnixListener), config)

	fatal(srv.Serve())
}
