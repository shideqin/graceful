package graceful

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

var (
	listenTCP       = make(map[string]net.Listener)
	listenUDP       = make(map[string]net.PacketConn)
	listenPtrOffset = make(map[string]uint)
)

//ListenTCP 监听TCP
func ListenTCP(network, addr string) (net.Listener, error) {
	var err error
	var flag = "tcp@" + addr
	// 设置监听器的监听对象（新建的或已存在的 socket 描述符）
	if os.Getenv("GRACEFUL_CONTINUE") == "true" {
		// 子进程监听父进程传递的 socket 描述符
		// 子进程的 0, 1, 2 是预留给标准输入、标准输出、错误输出
		// 子进程 3 开始传递 socket 描述符
		for i, f := range strings.Split(os.Getenv("GRACEFUL_SOCKET"), ",") {
			listenPtrOffset[f] = uint(i)
		}
		f := os.NewFile(uintptr(3+listenPtrOffset[flag]), "")
		listenTCP[flag], err = net.FileListener(f)
	} else {
		// 父进程监听新建的 socket 描述符
		listenPtrOffset[flag] = uint(len(listenTCP) + len(listenUDP))
		listenTCP[flag], err = net.Listen(network, addr)
	}
	return listenTCP[flag], err
}

//ListenUDP 监听UDP
func ListenUDP(network, addr string) (net.PacketConn, error) {
	var err error
	var flag = "udp@" + addr
	// 设置监听器的监听对象（新建的或已存在的 socket 描述符）
	if os.Getenv("GRACEFUL_CONTINUE") == "true" {
		// 子进程监听父进程传递的 socket 描述符
		// 子进程的 0, 1, 2 是预留给标准输入、标准输出、错误输出
		// 子进程 3 开始传递 socket 描述符
		for i, f := range strings.Split(os.Getenv("GRACEFUL_SOCKET"), ",") {
			listenPtrOffset[f] = uint(i)
		}
		f := os.NewFile(uintptr(3+listenPtrOffset[flag]), "")
		listenUDP[flag], err = net.FilePacketConn(f)
	} else {
		// 父进程监听新建的 socket 描述符
		listenPtrOffset[flag] = uint(len(listenTCP) + len(listenUDP))
		listenUDP[flag], err = net.ListenPacket(network, addr)
	}
	return listenUDP[flag], err
}

func HandleSignal(fn func(ctx context.Context)) {
	sig := make(chan os.Signal, 1)
	// 监听信号
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		ctx := context.Background()
		switch <-sig {
		case syscall.SIGINT, syscall.SIGTERM: // 终止进程执行
			signal.Stop(sig)
			fn(ctx)
			return
		case syscall.SIGHUP: // 进程热重启
			err := reload()
			if err != nil {
				log.Fatalf("HandleSignal.SIGHUP reload: %v\n", err)
			}
			fn(ctx)
			return
		}
	}
}

func reload() error {
	var total = len(listenPtrOffset)
	var files = make([]*os.File, total)
	var serves = make([]string, total)

	err := reloadTCP(files, serves)
	if err != nil {
		return err
	}
	err = reloadUDP(files, serves)
	if err != nil {
		return err
	}
	var env = append(os.Environ(), "GRACEFUL_CONTINUE=true")
	if len(serves) > 0 {
		env = append(env, fmt.Sprintf("GRACEFUL_SOCKET=%s", strings.Join(serves, ",")))
	}
	return command(files, env)
}

func reloadTCP(files []*os.File, serves []string) error {
	var rErr error
	for flag, index := range listenPtrOffset {
		if !strings.Contains(flag, "tcp@") {
			continue
		}
		ln, ok := listenTCP[flag].(*net.TCPListener)
		if !ok {
			rErr = errors.New("listener is not tcp listener")
			break
		}
		// 获取 socket 描述符
		f, err := ln.File()
		if err != nil {
			rErr = err
			break
		}
		files[index] = f
		serves[index] = flag
	}
	return rErr
}

func reloadUDP(files []*os.File, serves []string) error {
	var rErr error
	for flag, index := range listenPtrOffset {
		if !strings.Contains(flag, "udp@") {
			continue
		}
		ln, ok := listenUDP[flag].(*net.UDPConn)
		if !ok {
			rErr = errors.New("listener is not udp listener")
			break
		}
		// 获取 socket 描述符
		f, err := ln.File()
		if err != nil {
			rErr = err
			break
		}
		files[index] = f
		serves[index] = flag
	}
	return rErr
}

func command(files []*os.File, env []string) error {
	// 设置传递给子进程的参数（包含 socket 描述符）
	cmd := exec.Command(os.Args[0])
	cmd.Stdout = os.Stdout // 标准输出
	cmd.Stderr = os.Stderr // 错误输出
	cmd.ExtraFiles = files // 文件描述符
	cmd.Env = env
	// 新建并执行子进程
	return cmd.Start()
}
