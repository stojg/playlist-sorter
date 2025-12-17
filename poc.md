# Playlist sorter

## Context

I like to play music on my speakers while I work in the morning or just hanging about at home. I preferable like listen to the music that I have bought and downloaded to avoid streaming from the Evil Empires. It's quite easy to just hit the random button, but the issue is that I like to have a playlist that starts a bit chiller and doesn't jump from ambient electronic to norwegian death metal all the time.

If I had all the energy and time in the world, I would make 100 song playlist (~6hr of music) every evening to listen to the next morning. But with the current state of phycsoaccustic and analysis of songs, I think we can automate parts of it so that every morning I can more or less hit a button and have my personal style of playlist.

## Tech overview

Digital music can be today be analyzed in several different ways. We can estimate the Energy levels (ambient vs grindcode), musical key (C-Majer vs F#-minor), Tempo (bpm), Genres, Artist, Albums and so on. 

The main idea is that we have a playlist of songs with all this metadata that we can score (via a ML fitness function), and reorder the playlist so the score gets lower and lower and in theory that would make a more harmonic playlist.

### Harmomic distance example

For example, we can use (western) music theory and say that a song in one key has a couple of naturally harmonic transitions.

E major -> E major
E major -> B major
E major -> A major
E major -> D-flat minor

This is the idea of the Camelot system, that takes this transitions into numbers that are easy to understarnd https://mixedinkey.com/harmonic-mixing-guide/

So we can put a cost to this 

Perfect Match:
  E major -> E major = 0

Good matches:
  E major -> B major = 1
  E major -> A major = 1
  E major -> D-flat minor = 1

Also a dramatic match would be:
 E Major -> E Minor = 2
 
All other combination is a score of 5

Now randomise the playlist, count up all the transition score and compare the score with the original one, keep the best.

### Problems and solutions

Randomising and finding a good solution is relatively fast and can be brute forced (but still NP hard with no guarantee of optiomal solution)

But if we take in consideration BPM, Energy levels, etc we soon get a multidimensional traveling salepersons problem.

So the idea is to use classic ML algorithm to minimise the cost function, and in my research I found that Genetic Algorithm (GA) is a reasonable solution to quite quickly find a decent local minumum. 

Allow the users to tweak the cost of the indiviudal parameters so they can personlise the playlist.

For example:
- Add cost if the same Artist or Album plays concurrently
- Add cost if we jump between widely different Genres
- Ignore harmonic mixing, but make sure we dont jump widely between BPMs


## Tech details / Prototype

Written in Golang, need speed, but don't need C++ or Rust speed, easy to make safe non allocation / multithreaded code that is readable.

Uses my own music library that has been analyzed with [Mixed In Key](https://mixedinkey.com/) that puts the key, bpm and energy into the song metadata.

I use beets to organise and try to ensure that the metadata and format of my music collection os correct. Beets has a ton of plugins, but it using free online music metadata sites like https://www.discogs.com/ and https://musicbrainz.org/ and makes sure the genres, artist and album is somewhat correct.

I have a couple of bash scripts that randomise a playlist 100 songs via beets, then run the playlist sorter and then syncs the playlist to my NAS.

I play this via homeserver NAS where I store all my music with https://www.music-assistant.io/ integrated with homeassistant.

https://github.com/stojg/playlist-sorter

