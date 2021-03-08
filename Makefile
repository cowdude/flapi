.PHONY:

default:
	cat Makefile

src: .PHONY
	rm -rf src/static
	go generate ./src/...

bin: src
	go build -o bin/server ./src

docker-image: bin Dockerfile
	docker build -t flapi .

run:
	docker run -it \
		-v /mnt/ml/flashlight:/data \
		-p 8855:8080 \
		--ipc=host \
		--runtime=nvidia \
		flapi server -v

run-production:
	docker run -it \
		-v /mnt/ml/flashlight:/data \
		-p 8855:8080 \
		--ipc=host \
		--runtime=nvidia \
		flapi server