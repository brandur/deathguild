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
dg-scraper

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

## Testing

Run the tests with:

    createdb deathguild-test
    make test

## Vendoring Dependencies

Dependencies are managed with govendor. New ones can be vendored using these
commands:

    go get -u github.com/kardianos/govendor
    govendor add +external

## Deployment

The site is deployed according to the [AWS Instrinsic Static Site][intrinsic]
with AWS CloudFlare and S3 and deployed automatically from Travis. Music
metadata is retrieved from Spotify's API. State is maintained inside of a
Postgres database on Heroku. Connection strings are in `.travis.yml` and
encrypted with `travis encrypt DATABASE_URL=...`. An AWS Lambda function
rebuilds `master` periodically.

* Public URL: https://deathguild.brandur.org
* CloudFront distribution ID: `ENEEJ6NCB4DP`
* S3 bucket: `deathguild-playlists`
* Production database: `PRODUCTION_URL` on app `deathguild-playlists`.
* Test database: `TEST_URL` on app `deathguild-playlists`.
* Lambda rebuild period: 4 hours
* Spotify account: `fyrerise`

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
