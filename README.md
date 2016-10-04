# deathguild [![Build Status](https://travis-ci.org/brandur/deathguild.svg?branch=master)](https://travis-ci.org/brandur/deathguild)

Death Guild is the oldest continually operating gothic/industrial dance club in
the United States, and second in the world. [More information here][wiki].

This app fetches playlists from the official Death Guild site, creates Spotify
versions of each playlists, and then deploys a static site where they can be
accessed. A [live version can be visited here][site].

## Setup

Setup the project using this set of commands:

``` sh
createdb deathguild
psql deathguild < db/structure.sql
make install

# you'll need to fill in Spotify credentials here
cp .env.sample .env

export $(cat .env)

# creates a database of all playlist/song information
dg-scrape

# tags songs with their Spotify IDs
dg-enrich-songs

# creates Spotify playlists (idempotent, so safe to run many times)
dg-create-playlists

# builds a static site linking the new playlists
dg-build

# serves the built static site so it can be viewed locally
dg-serve

# deploy the built site to S3
export AWS_ACCESS_KEY_ID=
export AWS_SECRET_ACCESS_KEY=
export S3_BUCKET=
make deploy
```

A `Procfile` provides a watch/rebuild/serve loop for iterating on the site:

    go get -u github.com/ddollar/forego
    forego start

## Deployment

The site is modeled after the [AWS Instrinsic Static Site][intrinsic] with AWS
CloudFlare and S3 and deployed automatically from Travis. Music metadata is
retrieved from Spotify's API. State is maintained inside of an ephemeral Travis
Postgres that's loaded from an S3 dump when a build starts and dumped when it
finishes. An AWS Lambda function rebuilds `master` periodically so that we can
stay up-to-date with new playlists.

* Public URL: https://deathguild.brandur.org
* CloudFront distribution ID: `ENEEJ6NCB4DP`
* S3 bucket: `deathguild-playlists`
* Database dump: `s3://deathguild-playlists/deathguild.sql`
* Lambda rebuild period: 4 hours
* Spotify account: `fyrerise`

## Databases

Deployments occur using ephmeral Postgres databases that last only as long as
the build does. However, builds dump the latest database state back into S3
after they finish, so it's easy to stand up a local mirror to run some
analytics:

``` sh
export DATABASE_NAME=deathguild
export DATABASE_URL=postgres://localhost/$DATABASE_NAME
export TARGET_DIR=./public

createdb $DATABASE_NAME
mkdir -p $TARGET_DIR
make database-fetch
make database-restore
```

## Testing

Run the tests with:

    createdb deathguild-test
    make test

## Vendoring Dependencies

Dependencies are managed with govendor. New ones can be vendored using these
commands:

    go get -u github.com/kardianos/govendor
    govendor add +external

## Notes

* Spotify has made procuring valid credentials (which are not unreasonably rate
  limited) for an app like this one quite annoying. I'd recommend creating a
  Spotify app to get a client ID/secret, and then using their [web API
  authentication examples app][spotify-example] to procure a usable refresh
  token (which this project will then use to get access tokens).

* Spotify's API rate limits are very aggressive and an initial backfill might
  take quite some time. However, all fetched state is remembered so a mostly
  up-to-date data set should see no rate limiting.

[intrinsic]: https://brandur.org/aws-intrinsic-static
[site]: https://deathguild.brandur.org
[spotify-example]: https://github.com/spotify/web-api-auth-examples
[wiki]: https://en.wikipedia.org/wiki/Death_Guild
