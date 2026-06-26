restart:
	docker compose -f deploy/docker-compose.separated.yml stop 
	docker compose -f deploy/docker-compose.separated.yml up -d --build