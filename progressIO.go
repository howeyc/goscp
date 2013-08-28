package main

import (
	"fmt"
	"io"
	"time"

	"github.com/howeyc/pb"
)

type writeProgress struct {
	embed    io.Writer
	progress *pb.ProgressBar
	start    time.Time
}

func (wp *writeProgress) Write(p []byte) (n int, err error) {
	n, err = wp.embed.Write(p)
	wp.progress.Add(n)
	return
}

func (wp *writeProgress) Close() error {
	dur := time.Since(wp.start)
	wp.progress.FinishPrint(fmt.Sprintf("File transfer completed with an average rate of %f kB/s", (float64(wp.progress.Total)/1024.0)/dur.Seconds()))
	if cls, ok := wp.embed.(io.Closer); ok {
		return cls.Close()
	}
	return nil
}

type readProgress struct {
	embed    io.Reader
	progress *pb.ProgressBar
	start    time.Time
}

func (rp *readProgress) Read(p []byte) (n int, err error) {
	n, err = rp.embed.Read(p)
	rp.progress.Add(n)
	return
}

func (rp *readProgress) Close() error {
	dur := time.Since(rp.start)
	rp.progress.FinishPrint(fmt.Sprintf("File transfer completed with an average rate of %f kB/s", (float64(rp.progress.Total)/1024.0)/dur.Seconds()))
	if cls, ok := rp.embed.(io.Closer); ok {
		return cls.Close()
	}
	return nil
}
