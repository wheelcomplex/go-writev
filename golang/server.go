package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
)

const (
	NbVideosInGroup = 512
	VideoSize       = 4096
	HeaderSize      = 12
)

var exit_signal int64 = 0

// sigHandle
func sigHandle(sigChan, exitChan chan os.Signal) {
	sig := <-sigChan
	fmt.Println("\nsignal catched:", sig)
	atomic.AddInt64(&exit_signal, 1)
	exitChan <- sig
}

func main() {
	var err error
	fmt.Println("golang version streaming server.")

	var port int
	var useWritev, writeOneByOne bool
	if len(os.Args) >= 3 {
		if port, err = strconv.Atoi(os.Args[1]); err != nil {
			panic(err)
		}
		useWritev = os.Args[2] == "true"
		writeOneByOne = false
	}
	if !useWritev && len(os.Args) >= 4 {
		writeOneByOne = os.Args[3] == "true"
	}
	if len(os.Args) < 3 || (!useWritev && len(os.Args) < 4) {
		fmt.Println("Usage:", os.Args[0], "<port> <use_writev> [write_one_by_one]")
		fmt.Println("   port: the tcp listen port.")
		fmt.Println("   use_writev: whether use writev. true or false.")
		fmt.Println("   write_one_by_one: for write(not writev), whether send packet one by one.")
		fmt.Println("Fox example:")
		fmt.Println("   ", os.Args[0], "1985 true")
		fmt.Println("   ", os.Args[0], "1985 false true")
		fmt.Println("   ", os.Args[0], "1985 false false")
		os.Exit(-1)
	}

	runtime.GOMAXPROCS(1)
	// cpp server is running on only one cpu
	fmt.Println("always use 1 cpu")

	fmt.Println(fmt.Sprintf("listen at tcp://%v, use writev %v", port, useWritev))
	if !useWritev {
		fmt.Println("for write, send one-by-one", writeOneByOne)
	}

	var addr *net.TCPAddr
	if addr, err = net.ResolveTCPAddr("tcp4", fmt.Sprintf("0.0.0.0:%v", port)); err != nil {
		fmt.Println("failed:", err)
		return
	}

	var listener *net.TCPListener
	if listener, err = net.ListenTCP("tcp", addr); err != nil {
		fmt.Println("failed:", err)
		return
	}
	defer listener.Close()

	var sigChan = make(chan os.Signal, 128)
	var exitChan = make(chan os.Signal, 128)
	//
	// incoming signals will be catched
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go sigHandle(sigChan, exitChan)

	go func() {
		<-exitChan
		listener.Close()
		fmt.Println("listener closed.")
	}()

	for atomic.LoadInt64(&exit_signal) == 0 {
		var conn *net.TCPConn
		if conn, err = listener.AcceptTCP(); err != nil {
			if atomic.LoadInt64(&exit_signal) > 0 {
				fmt.Println("listener exit for", err)
				break
			} else {
				panic(err)
			}
		}

		if err := conn.SetLinger(0); err != nil {
			panic(err)
		}

		// go connHandle(conn, useWritev, writeOneByOne)
		connHandle(conn, useWritev, writeOneByOne)
	}
}

func connHandle(c *net.TCPConn, useWritev, writeOneByOne bool) {
	defer c.Close()

	c.SetNoDelay(false)

	// use write, to avoid lots of syscall, we copy to a big buffer.
	buf := make([]byte, NbVideosInGroup*(HeaderSize+VideoSize))

	// @remark for test, each video is M bytes.
	video := make([]byte, VideoSize)

	// @remark for test, each video header is M0 bytes.
	header := make([]byte, HeaderSize)

	// @remark for test, each group contains N (header+video)s.
	group := make([][]byte, 2*NbVideosInGroup)

	for i := 0; i < 2*NbVideosInGroup; i += 2 {
		group[i] = header
		group[i+1] = video
	}

	// assume there is a video stream, which contains infinite video packets,
	// server must delivery all video packets to client.
	// for high performance, we send a group of video(to avoid too many syscall),
	// here we send 10 videos as a group.
	for atomic.LoadInt64(&exit_signal) == 0 {
		if useWritev {
			// sendout the video group by writev
			/*
				golang version streaming server.
				always use 1 cpu
				listen at tcp://1985, use writev true
				calling into writev: 1024
				exited from writev: 1024
				calling into writev: 1024

				Killed
			*/

			// BUG: Writev never return
			for i := 0; i < 2*NbVideosInGroup; i += 2 {
				group[i] = header
				group[i+1] = video
			}
			fmt.Println("calling into writev:", len(group))
			if _, err := c.Writev(group); err != nil {
				fmt.Println("failed:", err)
				return
			}
			fmt.Println("exited from writev:", len(group))
			continue
		}
		// sendout the video group.
		if err := srs_send(c, group, writeOneByOne, buf); err != nil {
			fmt.Println("failed:", err)
			return
		}
	}
	if atomic.LoadInt64(&exit_signal) > 0 {
		fmt.Println("connHandle exit for exit_signal ", atomic.LoadInt64(&exit_signal))
	}
	return
}

// each group contains N (header+video)s.
//      header is M bytes.
//      videos is M0 bytes.
func srs_send(conn *net.TCPConn, group [][]byte, writeOneByOne bool, buf []byte) (err error) {

	// use write, send one by one packet.
	// @remark avoid memory copy, but with lots of syscall, hurts performance.
	if writeOneByOne {
		for i := 0; i < 2*NbVideosInGroup; i++ {
			if _, err = conn.Write(group[i]); err != nil {
				return
			}
		}
		return
	}

	var nn int
	for i := 0; i < 2*NbVideosInGroup; i++ {
		b := group[i]
		copy(buf[nn:nn+len(b)], b)
		nn += len(b)
	}

	if _, err = conn.Write(buf); err != nil {
		return
	}
	return
}
