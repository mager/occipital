// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "license": {
            "name": "Apache 2.0",
            "url": "http://www.apache.org/licenses/LICENSE-2.0.html"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/discover": {
            "post": {
                "description": "Get the best content",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Home page",
                "parameters": [
                    {
                        "description": "Request parameters",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/discover.DiscoverRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/discover.DiscoverResponse"
                        }
                    }
                }
            }
        },
        "/profile": {
            "get": {
                "description": "Get profile details by user ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Get profile by ID",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User ID",
                        "name": "id",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/profile.ProfileResponse"
                        }
                    }
                }
            }
        },
        "/spotify/get_featured_tracks": {
            "get": {
                "description": "Get the top featured tracks on Spotify",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Spotify"
                ],
                "summary": "Get featured tracks on Spotify",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/spotify.GetFeaturedTracksResponse"
                        }
                    }
                }
            }
        },
        "/spotify/recommended_tracks": {
            "post": {
                "description": "Get the top featured tracks on Spotify",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Spotify"
                ],
                "summary": "Get recommended tracks on Spotify",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/spotify.RecommendedTracksResponse"
                        }
                    }
                }
            }
        },
        "/spotify/search": {
            "post": {
                "description": "Search for tracks on Spotify using a query string.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Spotify"
                ],
                "summary": "Search Spotify for tracks",
                "parameters": [
                    {
                        "description": "Search query",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/spotify.SearchRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/spotify.SearchResponse"
                        }
                    }
                }
            }
        },
        "/track": {
            "get": {
                "description": "Get track",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Get track",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Source ID",
                        "name": "sourceId",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Source",
                        "name": "source",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/track.GetTrackResponse"
                        }
                    }
                }
            }
        },
        "/user": {
            "put": {
                "description": "Update user details by user ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Update user by ID",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User ID",
                        "name": "id",
                        "in": "query",
                        "required": true
                    },
                    {
                        "description": "Updated user information",
                        "name": "user",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/database.User"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/user.UserResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "database.User": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "integer"
                },
                "username": {
                    "type": "string"
                }
            }
        },
        "discover.DiscoverRequest": {
            "type": "object",
            "properties": {
                "popular": {
                    "type": "integer"
                }
            }
        },
        "discover.DiscoverResponse": {
            "type": "object",
            "properties": {
                "tracks": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.Track"
                    }
                },
                "updated": {
                    "type": "string"
                }
            }
        },
        "occipital.Track": {
            "type": "object",
            "properties": {
                "analysis": {
                    "$ref": "#/definitions/occipital.TrackAnalysis"
                },
                "artist": {
                    "type": "string"
                },
                "features": {
                    "$ref": "#/definitions/occipital.TrackFeatures"
                },
                "genres": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "image": {
                    "type": "string"
                },
                "instruments": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.TrackInstrumentArtists"
                    }
                },
                "isrc": {
                    "type": "string"
                },
                "meta": {
                    "$ref": "#/definitions/occipital.TrackMeta"
                },
                "name": {
                    "type": "string"
                },
                "production_credits": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.TrackProductionCredit"
                    }
                },
                "release_date": {
                    "type": "string"
                },
                "song_credits": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.TrackSongCredit"
                    }
                },
                "source": {
                    "type": "string"
                },
                "source_id": {
                    "type": "string"
                }
            }
        },
        "occipital.TrackAnalysis": {
            "type": "object",
            "properties": {
                "duration": {
                    "type": "number"
                },
                "segments": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.TrackAnalysisSegment"
                    }
                }
            }
        },
        "occipital.TrackAnalysisSegment": {
            "type": "object",
            "properties": {
                "confidence": {
                    "description": "Confidence, from 0.0 to 1.0, of the reliability of the segmentation. Segments of the song which are difficult to logically segment (e.g: noise) may correspond to low values in this field.",
                    "type": "number"
                },
                "duration": {
                    "description": "Duration is the duration (in seconds) of the segment.",
                    "type": "number"
                },
                "loudness_end": {
                    "description": "LoudnessEnd is the offset loudness of the segment in decibels (dB). This value should be equivalent to the loudness_start of the following segment.",
                    "type": "number"
                },
                "loudness_max": {
                    "description": "LoudnessMax is the peak loudness of the segment in decibels (dB). Combined with loudness_start and loudness_max_time, these components can be used to describe the \"attack\" of the segment.",
                    "type": "number"
                },
                "loudness_start": {
                    "description": "LoudnessStart is the onset loudness of the segment in decibels (dB). Combined with loudness_max and loudness_max_time, these components can be used to describe the \"attack\" of the segment.",
                    "type": "number"
                },
                "pitches": {
                    "description": "Pitches are given by a “chroma” vector, corresponding to the 12 pitch classes C, C#, D to B, with values ranging from 0 to 1 that describe the relative dominance of every pitch in the chromatic scale. For example a C Major chord would likely be represented by large values of C, E and G (i.e. classes 0, 4, and 7). Vectors are normalized to 1 by their strongest dimension, therefore noisy sounds are likely represented by values that are all close to 1, while pure tones are described by one value at 1 (the pitch) and others near 0. As can be seen below, the 12 vector indices are a combination of low-power spectrum values at their respective pitch frequencies.",
                    "type": "array",
                    "items": {
                        "type": "number"
                    }
                },
                "start": {
                    "description": "Start is the starting point (in seconds) of the segment",
                    "type": "number"
                },
                "timbres": {
                    "description": "Timbres are the quality of a musical note or sound that distinguishes different types of musical instruments, or voices. It is a complex notion also referred to as sound color, texture, or tone quality, and is derived from the shape of a segment’s spectro-temporal surface, independently of pitch and loudness. The timbre feature is a vector that includes 12 unbounded values roughly centered around 0. Those values are high level abstractions of the spectral surface, ordered by degree of importance. For completeness however, the first dimension represents the average loudness of the segment; second emphasizes brightness; third is more closely correlated to the flatness of a sound; fourth to sounds with a stronger attack; etc. See an image below representing the 12 basis functions (i.e. template segments). The actual timbre of the segment is best described as a linear combination of these 12 basis functions weighted by the coefficient values: timbre = c1 x b1 + c2 x b2 + ... + c12 x b12, where c1 to c12 represent the 12 coefficients and b1 to b12 the 12 basis functions as displayed below. Timbre vectors are best used in comparison with each other.",
                    "type": "array",
                    "items": {
                        "type": "number"
                    }
                }
            }
        },
        "occipital.TrackFeatures": {
            "type": "object",
            "properties": {
                "acousticness": {
                    "description": "Acousticness is a confidence measure from 0.0 to 1.0 of whether the track is acoustic.\n1.0 represents high confidence the track is acoustic.\nExample: 0.00242",
                    "type": "number"
                },
                "danceability": {
                    "description": "Danceability describes how suitable a track is for dancing based on a combination of\nmusical elements including tempo, rhythm stability, beat strength, and overall regularity.\nA value of 0.0 is least danceable and 1.0 is most danceable.\nExample: 0.585",
                    "type": "number"
                },
                "energy": {
                    "description": "Energy is a measure from 0.0 to 1.0 and represents a perceptual measure of intensity\nand activity. Typically, energetic tracks feel fast, loud, and noisy. For example,\ndeath metal has high energy, while a Bach prelude scores low on the scale.\nPerceptual features contributing to this attribute include dynamic range, perceived\nloudness, timbre, onset rate, and general entropy.\nExample: 0.842",
                    "type": "number"
                },
                "happiness": {
                    "description": "Happiness is a measure from 0.0 to 1.0 describing the musical positiveness conveyed by a track. Tracks with high valence sound more positive (e.g. happy, cheerful, euphoric),\nwhile tracks with low valence sound more negative (e.g. sad, depressed, angry).\nRange: 0 - 1\nExample: 0.428",
                    "type": "number"
                },
                "instrumentalness": {
                    "description": "Instrumentalness predicts whether a track contains no vocals. \"Ooh\" and \"aah\" sounds are treated as instrumental in this context.\nRap or spoken word tracks are clearly \"vocal\". The closer the instrumentalness value is to 1.0, the greater likelihood the track contains no vocal content.\nValues above 0.5 are intended to represent instrumental tracks, but confidence is higher as the value approaches 1.0.\nExample: 0.00686",
                    "type": "number"
                },
                "liveness": {
                    "description": "Liveness detects the presence of an audience in the recording. Higher liveness values represent an increased probability that the track was performed live.\nA value above 0.8 provides strong likelihood that the track is live.\nExample: 0.0866",
                    "type": "number"
                },
                "loudness": {
                    "description": "Loudness is the overall loudness of a track in decibels (dB). Loudness values are averaged across the entire track and are useful for comparing relative loudness of tracks.\nLoudness is the quality of a sound that is the primary psychological correlate of physical strength (amplitude). Values typically range between -60 and 0 db.\nExample: -5.883",
                    "type": "number"
                },
                "speechiness": {
                    "description": "Speechiness detects the presence of spoken words in a track. The more exclusively speech-like the recording (e.g. talk show, audio book, poetry), the closer to 1.0 the attribute value.\nValues above 0.66 describe tracks that are probably made entirely of spoken words. Values between 0.33 and 0.66 describe tracks that may contain both music and speech, either in sections or layered,\nincluding such cases as rap music. Values below 0.33 most likely represent music and other non-speech-like tracks.\nExample: 0.0556",
                    "type": "number"
                }
            }
        },
        "occipital.TrackInstrumentArtists": {
            "type": "object",
            "properties": {
                "artists": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "instrument": {
                    "type": "string"
                }
            }
        },
        "occipital.TrackMeta": {
            "type": "object",
            "properties": {
                "duration_ms": {
                    "description": "DurationMs is the duration of the track in milliseconds.\nExample: 237040",
                    "type": "integer"
                },
                "key": {
                    "description": "Key is the key the track is in. Integers map to pitches using standard Pitch Class notation. E.g. 0 = C, 1 = C♯/D♭, 2 = D, and so on. If no key was detected, the value is -1.\nRange: -1 - 11\nExample: 9",
                    "type": "integer"
                },
                "mode": {
                    "description": "Mode indicates the modality (major or minor) of a track, the type of scale from which its melodic content is derived.\nMajor is represented by 1 and minor is 0.\nExample: 0",
                    "type": "integer"
                },
                "tempo": {
                    "description": "Tempo is the overall estimated tempo of a track in beats per minute (BPM). In musical terminology, tempo is the speed or pace of a given piece and derives directly from the average beat duration.\nExample: 118.211",
                    "type": "number"
                },
                "time_signature": {
                    "description": "TimeSignature is an estimated time signature. The time signature (meter) is a notational convention to specify how many beats are in each bar (or measure).\nThe time signature ranges from 3 to 7 indicating time signatures of \"3/4\", to \"7/4\".\nRange: 3 - 7\nExample: 4",
                    "type": "integer"
                }
            }
        },
        "occipital.TrackProductionCredit": {
            "type": "object",
            "properties": {
                "artists": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "credit": {
                    "type": "string"
                }
            }
        },
        "occipital.TrackSongCredit": {
            "type": "object",
            "properties": {
                "artists": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "credit": {
                    "type": "string"
                }
            }
        },
        "profile.ProfileResponse": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "integer"
                },
                "username": {
                    "type": "string"
                }
            }
        },
        "spotify.FeaturedTrack": {
            "type": "object",
            "properties": {
                "artist": {
                    "type": "string"
                },
                "image": {
                    "type": "string"
                },
                "name": {
                    "type": "string"
                },
                "source": {
                    "type": "string"
                },
                "source_id": {
                    "type": "string"
                }
            }
        },
        "spotify.GetFeaturedTracksResponse": {
            "type": "object",
            "properties": {
                "tracks": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/spotify.FeaturedTrack"
                    }
                }
            }
        },
        "spotify.RecommendedTracksResponse": {
            "type": "object",
            "properties": {
                "tracks": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/occipital.Track"
                    }
                }
            }
        },
        "spotify.SearchRequest": {
            "type": "object",
            "properties": {
                "limit": {
                    "type": "integer"
                },
                "query": {
                    "type": "string"
                }
            }
        },
        "spotify.SearchResponse": {
            "type": "object",
            "properties": {
                "results": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/spotify.SearchTrack"
                    }
                }
            }
        },
        "spotify.SearchTrack": {
            "type": "object",
            "properties": {
                "artist": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "name": {
                    "type": "string"
                },
                "popularity": {
                    "type": "integer"
                },
                "thumb": {
                    "type": "string"
                }
            }
        },
        "track.GetTrackResponse": {
            "type": "object",
            "properties": {
                "track": {
                    "$ref": "#/definitions/occipital.Track"
                }
            }
        },
        "user.UserResponse": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "integer"
                },
                "username": {
                    "type": "string"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:8080",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "Occipital",
	Description:      "This is the API for occipital",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
