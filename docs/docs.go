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
        }
    },
    "definitions": {
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
