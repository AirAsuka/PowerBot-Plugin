go version
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GO111MODULE=auto
go mod tidy
#go build -ldflags="-s -w" -o ZeroBot-Plugin
go generate main.go

# 日志配置
LOGDIR="logs"
mkdir -p "$LOGDIR"
LOGFILE="$LOGDIR/$(date +%Y%m%d_%H%M%S).log"

# 运行并保存日志到文件，同时输出到控制台
go run -ldflags "-s -w" main.go 2>&1 | tee "$LOGFILE"
