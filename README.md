# Explore Service

## Overview

This is a small subset of The Explore Service, a gRPC microservice responsible for managing user interaction decisions on the Muzz platform.  
Users can either **like** or **pass** on other users, and the service tracks these decisions and mutual likes.  
This service implements four gRPC endpoints to list likers, count likers, and record decisions.

---

## Proto Definition Summary

The service conforms to the following gRPC endpoints:

- `ListLikedYou(ListLikedYouRequest) → ListLikedYouResponse`  
  Lists users who liked the recipient.

- `ListNewLikedYou(ListLikedYouRequest) → ListLikedYouResponse`  
  Lists users who liked the recipient excluding those already liked back.

- `CountLikedYou(CountLikedYouRequest) → CountLikedYouResponse`  
  Counts users who liked the recipient.

- `PutDecision(PutDecisionRequest) → PutDecisionResponse`  
  Records the actor's decision to like or pass the recipient. Returns whether there's a mutual like.

Message structure includes pagination tokens for scalable listing.

---

## Architecture & Design

- **Language & tools:** Written in Go using the latest Go 1.24 version and gRPC/protobuf tooling.
- **Data storage:** PostgreSQL is used as the primary persistent store for decisions. Redis is used as a cache layer with TTL for improved performance.
- **Metrics:** Prometheus metrics are exposed via a dedicated HTTP endpoint.
- **Containerization:** Fully containerized with Docker and orchestrated using Docker Compose. Dependencies include Postgres, Redis, Prometheus, and Grafana, and Adminer.
- **Scalability:**
  - Pagination and caching used to efficiently handle users with large decision sets.
  - Decisions can be overwritten to keep data consistent.
  - Designed for high throughput with latency metrics tracked.
- **Logging:** Structured logging with Zap.

---

## Building and Running

### Prerequisites

- Docker and Docker Compose installed
- Ports 8080, 2112, 9001, 5432, 6379, 9090, and 3000 free (or adjusted accordingly)

## Environment Variables

The service uses several environment variables to configure connections, timeouts, and ports. Reasonable default values are set but you can override them by setting these variables in your environment or Docker compose.

| Variable            | Default     | Description                                       |
| ------------------- | ----------- | ------------------------------------------------- |
| `REDIS_HOST`        | `localhost` | Redis server hostname                             |
| `REDIS_PORT`        | `6379`      | Redis server port                                 |
| `REDIS_PASSWORD`    | _(empty)_   | Redis password if required                        |
| `POSTGRES_HOST`     | `localhost` | PostgreSQL server hostname                        |
| `POSTGRES_PORT`     | `5432`      | PostgreSQL server port                            |
| `POSTGRES_USER`     | `postgres`  | PostgreSQL username                               |
| `POSTGRES_PASSWORD` | `password`  | PostgreSQL password                               |
| `POSTGRES_DB`       | `myappdb`   | PostgreSQL database name                          |
| `POSTGRES_RETRY`    | `10`        | Number of retry attempts to connect to PostgreSQL |
| `POSTGRES_DELAY`    | `2s`        | Delay between PostgreSQL retry attempts           |
| `EXPLORE_TTL`       | `30s`       | TTL for explore data cache                        |
| `PROMETHEUS_PORT`   | `2112`      | Port to expose Prometheus metrics                 |
| `GRPC_PORT`         | `9001`      | Port for the gRPC server                          |
| `PAGE_SIZE`         | `25`        | Default page size for paginated list responses    |

You can set these in your Docker Compose file under the `environment` section or export them in your shell before running the service.

---

## The Problem of Scalability

One of the interesting challenges with this task is figuring out how best to make the application scalable. As mentioned in the brief, some users may have hundreds of thousands
of likes accumulated over many years. Obviously, returning all of these in a ListLikedYouRequest is not feasible, so other options have to be considered. Furthermore there may be some users who refresh the app very frequently to see their new likes, or their count of likes. Without some kind of intervention, each refresh would hit the database - and depending on how many likes they have, this operation could prove quite costly from a resource perspective.

### Solutions

In this coding challenge I've experimented with a number of solutions and settled on a few which I felt were feasible given the time constraints. Firstly the matter of very popular users who have accumulated a high volume of likes. Each time they refresh to see the number of new likes or hit the ListNewLikedYouRequest endpoint, could put moderate strain on resources. For this I have simply opted to limit results returned to those decisions made within the past year. Now this period of time could change, but I felt a year was reasonable. In addition, I've implemented caching with redis, for the ListLikedYouRequest, ListNewLikedYouRequest, and the CountLikedYouRequest endpoints, with a TTL of 30s (configurable via environment variables). The caching would also significantly limit the impact of high frequency refreshing from users. The TTL for these three endpoints is the same so as not to create a disconnect between the number of likes visibly seen by the user, and the number shown as a value.

### Pagination

Related to the problem of scalability is pagination. Fortunately the protobuf definition included pagination tokens, so this was trivial to implement. So as not to overwhelm the users device, a default value of 25 has been chosen (configurable via environment variables).

### Build and run locally with Docker Compose

```bash
docker-compose up --build

```

## Building and Running

- Once running, you can access the logs by using:

```bash
docker logs -f "explore-service"
```

- Your gRPC client will be able to communicate with the server via localhost:9001 if running locally, or via explore-server:9001 if on the same docker network.

## Data

- The table schemas are as follows:

```
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE likes (
    id SERIAL PRIMARY KEY,
    liker_id TEXT NOT NULL,
    liked_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    decision BOOLEAN, -- Using true / false for whether the actor likes the recipient.
    decision_at TIMESTAMP,
    UNIQUE (liker_id, liked_id) -- This enables updating of a row when liker_id and liked_id match the existing row.
);

CREATE INDEX idx_likes_liked_id ON likes (liked_id);
CREATE INDEX idx_likes_liker_id ON likes (liker_id);
CREATE INDEX idx_likes_liked_created ON likes (liked_id, created_at DESC);
```

- The docker-compose file will run the init.sql to populate the tables on building the image. The users table has 55 users, roughly evenly split by gender.
- Usernames are simply 'userX' where X is a number, e.g user1, user34, and so on.
- The likes table is comprised of 40 mutual likes, 86 one-way likes, and 12 "dislikes". The assumption in this case being that a decision does not exist (null) until it is actively made. So a decision status of "false" implies an active dislike.
- On average, users have liked 2 to 4 other users. For the purpose of testing pagination, user55 has had 26 likes, none reciprocated.
- If you want to view the tables, Adminer can be accessed at localhost:5050. Server and username are postgres, password=password, database=myappdb.

### Observability

## Prometheus Metrics

The service exposes the following Prometheus metrics on port `2112` (configurable via `PROMETHEUS_PORT`). These metrics provide insight into request counts, errors, mutual matches, and request durations for each endpoint.

| Metric Name                                                    | Type      | Description                                                      |
| -------------------------------------------------------------- | --------- | ---------------------------------------------------------------- |
| `explore-service_list_liked_you_requests_total`                | Counter   | Total number of `ListLikedYou` requests                          |
| `explore-service_list_liked_you_request_errors_total`          | Counter   | Total number of errors from `ListLikedYou` requests              |
| `explore-service_list_liked_you_requests_duration_seconds`     | Histogram | Duration of `ListLikedYou` requests in seconds                   |
| `explore-service_list_new_liked_you_requests_total`            | Counter   | Total number of `ListNewLikedYou` requests                       |
| `explore-service_list_new_liked_you_request_errors_total`      | Counter   | Total number of errors from `ListNewLikedYou` requests           |
| `explore-service_list_new_liked_you_requests_duration_seconds` | Histogram | Duration of `ListNewLikedYou` requests in seconds                |
| `explore-service_count_liked_you_requests_total`               | Counter   | Total number of `CountLikedYou` requests                         |
| `explore-service_count_liked_you_request_errors_total`         | Counter   | Total number of errors from `CountLikedYou` requests             |
| `explore-service_count_liked_you_requests_duration_seconds`    | Histogram | Duration of `CountLikedYou` requests in seconds                  |
| `explore-service_put_decision_requests_total`                  | Counter   | Total number of `PutDecision` requests                           |
| `explore-service_put_decision_request_errors_total`            | Counter   | Total number of errors from `PutDecision` requests               |
| `explore-service_put_decision_requests_duration_seconds`       | Histogram | Duration of `PutDecision` requests in seconds                    |
| `explore-service_put_decision_mutual_matches_total`            | Counter   | Total number of mutual likes (i.e., both users liked each other) |
| `explore-service_put_decision_no_mutual_matches_total`         | Counter   | Total number of non-mutual likes                                 |

The output of the metrics can be viewed from localhost:2112/metrics, or from Prometheus on lcaolhost:9090 and searched like this:

```
{"explore-service_count_liked_you_requests_total",  request="count_liked_you_requests_total"}
```

Alternatively...

## Grafana

I've also provided a basic Grafana dashboard where these metrics can be viewed. You can access this by going to localhost:3000, and using the login details:

- username: admin
- password: muzzd8

The dashboard can be found in the dashboards section, and is named Explore-Service-Aaron-King.

### Final thoughts

There are a few things I would have liked to have done, which, had I had more time, could have improved the end user experience. The idea of using an archive database is one of them. The idea is that for each user, once they get to X number of likes (say 1000 perhaps), the oldest likes over number 1000 are migrated to an archive database. Another feature I that could have led to an improved experience overall, is rate limiting. While my implementation of caching means frequent refereshers don't hit the database so often, it doesn't stop them hitting the server with requests - so rate limiting may have helped there, also for bad actors. Still on the theme of scalability, having database duplication with load balancing is another way to improve the user experience. The basic idea would be to have a main write DB, and perhaps 3 or more duplicates which are for read only. Updates to the read only databases would be handled accordingly, and reads could be distributed using a load balancer (perhaps something like nginx). In conclusion there are myriad ways in which to approach scalability, but hopefully I've demonstrated a suitable variety of appropriate techniques for the time that I've had.
