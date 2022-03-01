all:
	docker build -t ipfs-check-pp . && docker run -p 3333:3333 -it ipfs-check-pp
