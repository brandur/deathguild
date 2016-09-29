# deathguild [![Build Status](https://travis-ci.org/brandur/deathguild.svg?branch=master)](https://travis-ci.org/brandur/deathguild)

## Setup

    createdb deathguild
    psql deathguild < db/structure.sql
    go install ./...
    cp .env.sample .env
    export $(cat .env)
    dg-scraper

## Architecture

`dg-scraper` -- scrapes the Death Guild website and stores information to Postgres.
`dg-spotify` -- uses Spotify to enrich database information and create playlists.

## Vendoring Dependencies

    go get -u github.com/kardianos/govendor
    govendor add +external
