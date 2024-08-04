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
        "/user/{id}": {
            "get": {
                "description": "Get user details by user ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Get user by ID",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/health.GetUserResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "health.GetUserResponse": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                }
            }
        },
        "occipital.Track": {
            "type": "object",
            "properties": {
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
                        "$ref": "#/definitions/occipital.TrackArtistInstruments"
                    }
                },
                "isrc": {
                    "type": "string"
                },
                "name": {
                    "type": "string"
                },
                "release_date": {
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
        "occipital.TrackArtistInstruments": {
            "type": "object",
            "properties": {
                "artist": {
                    "type": "string"
                },
                "instruments": {
                    "type": "array",
                    "items": {
                        "type": "string"
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
                "duration_ms": {
                    "description": "DurationMs is the duration of the track in milliseconds.\nExample: 237040",
                    "type": "integer"
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
                "key": {
                    "description": "Key is the key the track is in. Integers map to pitches using standard Pitch Class notation. E.g. 0 = C, 1 = C♯/D♭, 2 = D, and so on. If no key was detected, the value is -1.\nRange: -1 - 11\nExample: 9",
                    "type": "integer"
                },
                "liveness": {
                    "description": "Liveness detects the presence of an audience in the recording. Higher liveness values represent an increased probability that the track was performed live.\nA value above 0.8 provides strong likelihood that the track is live.\nExample: 0.0866",
                    "type": "number"
                },
                "loudness": {
                    "description": "Loudness is the overall loudness of a track in decibels (dB). Loudness values are averaged across the entire track and are useful for comparing relative loudness of tracks.\nLoudness is the quality of a sound that is the primary psychological correlate of physical strength (amplitude). Values typically range between -60 and 0 db.\nExample: -5.883",
                    "type": "number"
                },
                "mode": {
                    "description": "Mode indicates the modality (major or minor) of a track, the type of scale from which its melodic content is derived.\nMajor is represented by 1 and minor is 0.\nExample: 0",
                    "type": "integer"
                },
                "speechiness": {
                    "description": "Speechiness detects the presence of spoken words in a track. The more exclusively speech-like the recording (e.g. talk show, audio book, poetry), the closer to 1.0 the attribute value.\nValues above 0.66 describe tracks that are probably made entirely of spoken words. Values between 0.33 and 0.66 describe tracks that may contain both music and speech, either in sections or layered,\nincluding such cases as rap music. Values below 0.33 most likely represent music and other non-speech-like tracks.\nExample: 0.0556",
                    "type": "number"
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
