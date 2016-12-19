linux-bufferlinks: *.go
	GOOS=linux go build -o linux-bufferlinks

image: linux-bufferlinks
	docker build . -t alexflint/bufferlinks

push: image
	docker push alexflint/bufferlinks:latest

run:
	docker run -it --rm -P alexflint/bufferlinks

terminal:
	docker run -it --rm --entrypoint /bin/sh alexflint/bufferlinks
