TEST_RESULT=0
DOCKERFILE='Dockerfile-test'
IMAGE='export'
TEARDOWN_RAN=0

teardown() {

    [ "$TEARDOWN_RAN" -ne "0" ] && return

    echo "Running teardown..."

    docker rm -f "$TEST_CONTAINER_NAME"
    TEARDOWN_RAN=1
}

trap teardown EXIT ERR SIGINT SIGTERM

mkdir -p artifacts

get_N_chars_commit_hash() {

    local CHARS=${1:-7}

    git rev-parse --short="$CHARS" HEAD
}

TEST_CONTAINER_NAME="export-$(get_N_chars_commit_hash 7)"

echo "Building image"
docker build -f "$DOCKERFILE" -t "$IMAGE" .

echo -e "\n---------------------------------------------------------------\n"

echo "Running container"
docker run -d --rm --name "$TEST_CONTAINER_NAME" "$IMAGE" sleep infinity

echo -e "\n---------------------------------------------------------------\n"

echo "Linting"
docker exec --workdir /workdir  --user 1001 -e PATH=/opt/app-root/src/go/bin:$PATH "$TEST_CONTAINER_NAME" golint -set_exit_status ./... > 'artifacts/linter_logs.txt'
TEST_RESULT=$?

cat artifacts/linter_logs.txt

echo -e "\n---------------------------------------------------------------\n"

if [ $TEST_RESULT -eq 0 ]; then
    echo "Linting ran successfully"
else
    echo "Linting failed..."
    sh "exit 1"
fi
