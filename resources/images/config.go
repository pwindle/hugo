// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package images

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/disintegration/gift"

	"github.com/mitchellh/mapstructure"
)

const (
	defaultJPEGQuality    = 75
	defaultResampleFilter = "box"
)

var (
	imageFormats = map[string]Format{
		".jpg":  JPEG,
		".jpeg": JPEG,
		".png":  PNG,
		".tif":  TIFF,
		".tiff": TIFF,
		".bmp":  BMP,
		".gif":  GIF,
	}

	// Add or increment if changes to an image format's processing requires
	// re-generation.
	imageFormatsVersions = map[Format]int{
		PNG: 2, // Floyd Steinberg dithering
	}

	// Increment to mark all processed images as stale. Only use when absolutely needed.
	// See the finer grained smartCropVersionNumber and imageFormatsVersions.
	mainImageVersionNumber = 0
)

var anchorPositions = map[string]gift.Anchor{
	strings.ToLower("Center"):      gift.CenterAnchor,
	strings.ToLower("TopLeft"):     gift.TopLeftAnchor,
	strings.ToLower("Top"):         gift.TopAnchor,
	strings.ToLower("TopRight"):    gift.TopRightAnchor,
	strings.ToLower("Left"):        gift.LeftAnchor,
	strings.ToLower("Right"):       gift.RightAnchor,
	strings.ToLower("BottomLeft"):  gift.BottomLeftAnchor,
	strings.ToLower("Bottom"):      gift.BottomAnchor,
	strings.ToLower("BottomRight"): gift.BottomRightAnchor,
}

var imageFilters = map[string]gift.Resampling{

	strings.ToLower("NearestNeighbor"):   gift.NearestNeighborResampling,
	strings.ToLower("Box"):               gift.BoxResampling,
	strings.ToLower("Linear"):            gift.LinearResampling,
	strings.ToLower("Hermite"):           hermiteResampling,
	strings.ToLower("MitchellNetravali"): mitchellNetravaliResampling,
	strings.ToLower("CatmullRom"):        catmullRomResampling,
	strings.ToLower("BSpline"):           bSplineResampling,
	strings.ToLower("Gaussian"):          gaussianResampling,
	strings.ToLower("Lanczos"):           gift.LanczosResampling,
	strings.ToLower("Hann"):              hannResampling,
	strings.ToLower("Hamming"):           hammingResampling,
	strings.ToLower("Blackman"):          blackmanResampling,
	strings.ToLower("Bartlett"):          bartlettResampling,
	strings.ToLower("Welch"):             welchResampling,
	strings.ToLower("Cosine"):            cosineResampling,
}

func ImageFormatFromExt(ext string) (Format, bool) {
	f, found := imageFormats[ext]
	return f, found
}

func DecodeConfig(m map[string]interface{}) (Imaging, error) {
	var i Imaging
	if err := mapstructure.WeakDecode(m, &i); err != nil {
		return i, err
	}

	if i.Quality == 0 {
		i.Quality = defaultJPEGQuality
	} else if i.Quality < 0 || i.Quality > 100 {
		return i, errors.New("JPEG quality must be a number between 1 and 100")
	}

	if i.Anchor == "" || strings.EqualFold(i.Anchor, smartCropIdentifier) {
		i.Anchor = smartCropIdentifier
	} else {
		i.Anchor = strings.ToLower(i.Anchor)
		if _, found := anchorPositions[i.Anchor]; !found {
			return i, errors.New("invalid anchor value in imaging config")
		}
	}

	if i.ResampleFilter == "" {
		i.ResampleFilter = defaultResampleFilter
	} else {
		filter := strings.ToLower(i.ResampleFilter)
		_, found := imageFilters[filter]
		if !found {
			return i, fmt.Errorf("%q is not a valid resample filter", filter)
		}
		i.ResampleFilter = filter
	}

	return i, nil
}

func DecodeImageConfig(action, config string, defaults Imaging) (ImageConfig, error) {
	var (
		c   ImageConfig
		err error
	)

	c.Action = action

	if config == "" {
		return c, errors.New("image config cannot be empty")
	}

	parts := strings.Fields(config)
	for _, part := range parts {
		part = strings.ToLower(part)

		if part == smartCropIdentifier {
			c.AnchorStr = smartCropIdentifier
		} else if pos, ok := anchorPositions[part]; ok {
			c.Anchor = pos
			c.AnchorStr = part
		} else if filter, ok := imageFilters[part]; ok {
			c.Filter = filter
			c.FilterStr = part
		} else if part[0] == 'q' {
			c.Quality, err = strconv.Atoi(part[1:])
			if err != nil {
				return c, err
			}
			if c.Quality < 1 || c.Quality > 100 {
				return c, errors.New("quality ranges from 1 to 100 inclusive")
			}
		} else if part[0] == 'r' {
			c.Rotate, err = strconv.Atoi(part[1:])
			if err != nil {
				return c, err
			}
		} else if strings.Contains(part, "x") {
			widthHeight := strings.Split(part, "x")
			if len(widthHeight) <= 2 {
				first := widthHeight[0]
				if first != "" {
					c.Width, err = strconv.Atoi(first)
					if err != nil {
						return c, err
					}
				}

				if len(widthHeight) == 2 {
					second := widthHeight[1]
					if second != "" {
						c.Height, err = strconv.Atoi(second)
						if err != nil {
							return c, err
						}
					}
				}
			} else {
				return c, errors.New("invalid image dimensions")
			}

		}
	}

	if c.Width == 0 && c.Height == 0 {
		return c, errors.New("must provide Width or Height")
	}

	if c.FilterStr == "" {
		c.FilterStr = defaults.ResampleFilter
		c.Filter = imageFilters[c.FilterStr]
	}

	if c.AnchorStr == "" {
		c.AnchorStr = defaults.Anchor
		if !strings.EqualFold(c.AnchorStr, smartCropIdentifier) {
			c.Anchor = anchorPositions[c.AnchorStr]
		}
	}

	return c, nil
}

// ImageConfig holds configuration to create a new image from an existing one, resize etc.
type ImageConfig struct {
	Action string

	// If set, this will be used as the key in filenames etc.
	Key string

	// Quality ranges from 1 to 100 inclusive, higher is better.
	// This is only relevant for JPEG images.
	// Default is 75.
	Quality int

	// Rotate rotates an image by the given angle counter-clockwise.
	// The rotation will be performed first.
	Rotate int

	Width  int
	Height int

	Filter    gift.Resampling
	FilterStr string

	Anchor    gift.Anchor
	AnchorStr string
}

func (i ImageConfig) GetKey(format Format) string {
	if i.Key != "" {
		return i.Action + "_" + i.Key
	}

	k := strconv.Itoa(i.Width) + "x" + strconv.Itoa(i.Height)
	if i.Action != "" {
		k += "_" + i.Action
	}
	if i.Quality > 0 {
		k += "_q" + strconv.Itoa(i.Quality)
	}
	if i.Rotate != 0 {
		k += "_r" + strconv.Itoa(i.Rotate)
	}
	anchor := i.AnchorStr
	if anchor == smartCropIdentifier {
		anchor = anchor + strconv.Itoa(smartCropVersionNumber)
	}

	k += "_" + i.FilterStr

	if strings.EqualFold(i.Action, "fill") {
		k += "_" + anchor
	}

	if v, ok := imageFormatsVersions[format]; ok {
		k += "_" + strconv.Itoa(v)
	}

	if mainImageVersionNumber > 0 {
		k += "_" + strconv.Itoa(mainImageVersionNumber)
	}

	return k
}

// Imaging contains default image processing configuration. This will be fetched
// from site (or language) config.
type Imaging struct {
	// Default image quality setting (1-100). Only used for JPEG images.
	Quality int

	// Resample filter to use in resize operations..
	ResampleFilter string

	// The anchor to use in Fill. Default is "smart", i.e. Smart Crop.
	Anchor string
}
