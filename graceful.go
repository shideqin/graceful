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
	listenTCP          = make(map[string]net.Listener)
	listenTCPPtrOffset = make(map[string]uint)
	listenUDP          = make(map[string]net.PacketConn)
	listenUDPPtrOffset = make(map[string]uint)
)

//ListenTCP 监听TCP
func ListenTCP(network, addr string) (net.Listener, error) {
	var err error
	// 设置监听器的监听对象（新建的或已存在的 socket 描述符）
	if os.Getenv("GRACEFUL-TCP-WORK") == "true" {
		// 子进程监听父进程传递的 socket 描述符
		// 子进程的 0, 1, 2 是预留给标准输入、标准输出、错误输出
		// 子进程 3 开始传递 socket 描述符
		for i, addr := range strings.Split(os.Getenv("GRACEFUL-TCP-SERVES"), ",") {
			listenTCPPtrOffset[addr] = uint(i)
		}
		f := os.NewFile(uintptr(3+listenTCPPtrOffset[addr]), "")
		listenTCP[addr], err = net.FileListener(f)
	} else {
		// 父进程监听新建的 socket 描述符
		listenTCP[addr], err = net.Listen(network, addr)
	}
	return listenTCP[addr], err
}

//ListenUDP 监听UDP
func ListenUDP(network, addr string) (net.PacketConn, error) {
	var err error
	// 设置监听器的监听对象（新建的或已存在的 socket 描述符）
	if os.Getenv("GRACEFUL-UDP-WORK") == "true" {
		// 子进程监听父进程传递的 socket 描述符
		// 子进程的 0, 1, 2 是预留给标准输入、标准输出、错误输出
		// 子进程 3 开始传递 socket 描述符
		for i, addr := range strings.Split(os.Getenv("GRACEFUL-UDP-SERVES"), ",") {
			listenTCPPtrOffset[addr] = uint(i)
		}
		f := os.NewFile(uintptr(3+listenTCPPtrOffset[addr]), "")
		listenUDP[addr], err = net.FilePacketConn(f)
	} else {
		// 父进程监听新建的 socket 描述符
		listenUDP[addr], err = net.ListenPacket(network, addr)
	}
	return listenUDP[addr], err
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
			err := reloadTCP()
			if err != nil {
				log.Fatalf("HandleSignal.SIGHUP reloadTCP: %v\n", err)
			}
			err = reloadUDP()
			if err != nil {
				log.Fatalf("HandleSignal.SIGHUP reloadUDP: %v\n", err)
			}
			fn(ctx)
			return
		}
	}
}

func reloadTCP() error {
	var files = make([]*os.File, len(listenTCPPtrOffset))
	var serves = make([]string, len(listenTCPPtrOffset))
	var rErr error
	for addr, i := range listenTCPPtrOffset {
		ln, ok := listenTCP[addr].(*net.TCPListener)
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
		files[i] = f
		serves[i] = addr
	}
	if rErr != nil {
		return rErr
	}
	var env = append(os.Environ(), "GRACEFUL-TCP-WORK=true")
	if len(serves) > 0 {
		env = append(env, fmt.Sprintf("GRACEFUL-TCP-SERVES=%s", strings.Join(serves, ",")))
	}
	return command(files, env)
}

func reloadUDP() error {
	var files = make([]*os.File, len(listenUDPPtrOffset))
	var serves = make([]string, len(listenUDPPtrOffset))
	var rErr error
	for addr, i := range listenUDPPtrOffset {
		ln, ok := listenUDP[addr].(*net.UDPConn)
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
		files[i] = f
		serves[i] = addr
	}
	if rErr != nil {
		return rErr
	}
	var env = append(os.Environ(), "GRACEFUL-UDP-WORK=true")
	if len(serves) > 0 {
		env = append(env, fmt.Sprintf("GRACEFUL-UDP-SERVES=%s", strings.Join(serves, ",")))
	}
	return command(files, env)
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
