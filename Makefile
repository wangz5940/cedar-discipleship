COMPOSE_FILE ?= deploy/docker-compose.separated.yml
ENV_FILE ?= .env
COMPOSE = docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)

.PHONY: restart down

restart:
	./scripts/init-deploy-env.sh >/dev/null
	$(COMPOSE) down
	GOPROXY=https://goproxy.cn,direct $(COMPOSE) up -d --build

down:
	$(COMPOSE) down
