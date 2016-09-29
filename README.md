# deathguild

## Setup

    createdb deathguild
    psql deathguild < db/structure.sql
    go install ./...
    export $(cat .env)
    dg-scraper

## Architecture

`dg-scraper` -- scrapes the Death Guild website and stores information to Postgres.
`dg-spotify` -- uses Spotify to enrich database information and create playlists.


