restart:
	docker compose -f deploy/docker-compose.separated.yml down
	GOPROXY=https://goproxy.cn,direct docker compose -f deploy/docker-compose.separated.yml up -d --build
