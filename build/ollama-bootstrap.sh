#!/bin/sh
# ollama-bootstrap.sh —— 容器启动时自动拉取默认模型（oneshot，跑完即退出）
# 由 supervisord 调用，在 ollama serve 启动后异步执行
set -e

MODELS_DIR="${OLLAMA_MODELS:-/data/ollama/models}"
PULL_FLAG="/data/ollama/.pulled"
DEFAULT_MODEL="${OLLAMA_DEFAULT_MODEL:-qwen2.5:1.5b}"

# 确保模型目录存在
mkdir -p "$MODELS_DIR"

# 若禁用自动拉取，直接退出
if [ "${OLLAMA_AUTO_PULL:-true}" != "true" ]; then
    echo "[ollama-bootstrap] 自动拉取已禁用 (OLLAMA_AUTO_PULL=false)，跳过"
    exit 0
fi

# 已拉取过的标记文件存在则跳过（避免每次重启都拉取）
if [ -f "$PULL_FLAG" ]; then
    echo "[ollama-bootstrap] 模型已就绪（标记文件存在），跳过拉取"
    exit 0
fi

# 等待 ollama 服务就绪（最多等待 60 秒）
echo "[ollama-bootstrap] 等待 ollama 服务就绪..."
READY=0
for i in $(seq 1 60); do
    if curl -sf http://127.0.0.1:11434/api/tags >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 1
done

if [ "$READY" -ne 1 ]; then
    echo "[ollama-bootstrap] ollama 服务 60 秒内未就绪，放弃自动拉取"
    exit 1
fi

# 检查模型是否已存在（可能用户手动拉过）
EXISTING=$(curl -sf http://127.0.0.1:11434/api/tags 2>/dev/null | jq -r '.models[].name' 2>/dev/null || echo "")
if echo "$EXISTING" | grep -q "$DEFAULT_MODEL"; then
    echo "[ollama-bootstrap] 模型 $DEFAULT_MODEL 已存在，标记完成"
    touch "$PULL_FLAG"
    exit 0
fi

# 后台拉取模型，日志写入文件，避免阻塞 supervisord
echo "[ollama-bootstrap] 首次启动，后台拉取模型 $DEFAULT_MODEL（约 4.7GB，取决于网络）"
echo "[ollama-bootstrap] 拉取日志: $MODELS_DIR/../pull.log"
nohup /usr/local/bin/ollama pull "$DEFAULT_MODEL" > /data/ollama/pull.log 2>&1 &
PULL_PID=$!

# 等待拉取完成（后台进程），完成后写入标记
(
    wait $PULL_PID
    if [ $? -eq 0 ]; then
        touch "$PULL_FLAG"
        echo "[ollama-bootstrap] 模型 $DEFAULT_MODEL 拉取完成" >> /data/ollama/pull.log
    else
        echo "[ollama-bootstrap] 模型拉取失败，请检查网络后重试" >> /data/ollama/pull.log
    fi
) &

echo "[ollama-bootstrap] 模型拉取已转入后台，PID=$PULL_PID"
echo "[ollama-bootstrap] 可用 'docker exec -it <容器> tail -f /data/ollama/pull.log' 查看进度"
exit 0
