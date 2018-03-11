#!/usr/bin/env bash
#create container if it does not exist
if [ -z "`docker ps -q -f name=dynago_test`" ]; then
	echo Starting container for testing...
	docker run \
		-v `pwd`:/go/src/dynago \
		-e AWS_ACCESS_KEY_ID=ABCD \
		-e AWS_SECRET_ACCESS_KEY=123ABC \
		-p 8000:8000 --name dynago_test --rm -d \
		appittome/go_dynamo:latest
fi
#run tests everytime something changes
while inotifywait -qq --exclude \..*\.sw[a-z] -e modify -r .; do
	echo "running test suite..."
	docker exec -t dynago_test go test -run "$1" ../../go/src/dynago
done
