#!/bin/sh
set -eu

export DB_DRIVER="${DB_DRIVER:-postgres}"

if [ "${IS_PROD:-}" = "1" ] && [ -z "${DATABASE_URL:-}" ]; then
	echo "DATABASE_URL is required in production" >&2
	exit 1
fi

if [ -n "${DATABASE_URL:-}" ]; then
	echo "Running database migrations"
	attempt=1
	while ! ./target/migrate up; do
		if [ "$attempt" -ge 30 ]; then
			echo "Database migrations failed after $attempt attempts" >&2
			exit 1
		fi
		echo "Database migrations failed; retrying in 2s ($attempt/30)" >&2
		attempt=$((attempt + 1))
		sleep 2
	done
else
	echo "DATABASE_URL is not set; skipping database migrations"
fi

exec "$@"
