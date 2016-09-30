# deathguild [![Build Status](https://travis-ci.org/brandur/deathguild.svg?branch=master)](https://travis-ci.org/brandur/deathguild)

## Setup

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

# creates Spotify playlists mirrors
dg-create-playlists

# builds a static site linking the new playlists
dg-build

# serves the built static site
dg-serve
```

## Vendoring Dependencies

    go get -u github.com/kardianos/govendor
    govendor add +external
