package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
)

// 依赖moss命令行，原moss生成pb桩代码命令行如下：
// moss gen proto -f path/xxxx.proto -o /path/
// 原命令不会自动删除原path下的老的桩代码，直接生成会导致生成错误，需先删除老代码。此工具优化这一点
var filename, output, name string

const MOSS_GEN_PROTO_CMD = "moss gen proto -f %s -o %s"

func init() {
	flag.StringVar(&filename, "f", "", "# proto file path (required).")
	flag.StringVar(&output, "o", "", "# proto go code generate dir path (required).")
	flag.StringVar(&name, "n", "", "# proto go code directory name (required).")
	flag.Parse()
}

func main() {
	if filename == "" || output == "" || name == "" {
		flag.Usage()
		return
	}

	/*if _, err := os.Stat(filename); err != nil {
		log.Fatalln("proto file path error: ", err)
	}
	if _, err := os.Stat(output); err != nil {
		log.Fatalln("output directory path error: ", err)
	}*/

	// 先删除对应的桩代码文件夹
	/*if err := os.Remove(output + name); err != nil {
		log.Fatalln("remove dir error: ", err)
	}*/
	// 执行moss protoc命令
	cmd := exec.Command(fmt.Sprintf(MOSS_GEN_PROTO_CMD, filename, output))
	// 输出流
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalln("stdoutpipe error: ", err)
	}
	defer stdout.Close()
	// 运行命令
	if err := cmd.Start(); err != nil {
		log.Fatalln("exec moss cmd error: ", err)
	}
	// 读取输出流
	stdoutBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		log.Fatalln("read stdout error: ", err)
	}
	fmt.Println(string(stdoutBytes))
}
