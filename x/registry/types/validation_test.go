package types_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/glass-harbor/protocol/x/registry/types"
)

const goodMagnet = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=jetty-wallet-1.0.0.zip"

func TestValidateSemver(t *testing.T) {
	cases := map[string]struct {
		in string
		ok bool
	}{
		"plain":            {"1.0.0", true},
		"prerelease":       {"1.0.0-beta.1", true},
		"build metadata":   {"1.0.0+abc", false},
		"pre+build":        {"1.0.0-rc.1+abc", false},
		"leading v":        {"v1.0.0", false},
		"two parts":        {"1.0", false},
		"leading zero":     {"01.0.0", false},
		"empty":            {"", false},
		"too long":         {strings.Repeat("1", 65) + ".0.0", false},
		"whitespace":       {" 1.0.0", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := types.ValidateSemver(tc.in, 64)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, types.ErrInvalidSemver)
			}
		})
	}
}

func TestValidateMagnet(t *testing.T) {
	cases := map[string]struct {
		in string
		ok bool
	}{
		"hex btih":          {goodMagnet, true},
		"base32 btih":       {"magnet:?xt=urn:btih:MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43U", true},
		"upper hex":         {"magnet:?xt=urn:btih:C12FE1C06BBA254A9DC9F519B335AA7C1367A88A", true},
		"second xt":         {"magnet:?xt=urn:sha1:abc&xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a", true},
		"btmh only":         {"magnet:?xt=urn:btmh:1220c12fe1c06bba254a9dc9f519b335aa7c1367a88ac12fe1c06bba254a9dc9f519b3", false},
		"http scheme":       {"http://example.com/?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a", false},
		"short hash":        {"magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88", false},
		"no xt":             {"magnet:?dn=foo", false},
		"empty":             {"", false},
		"too long":          {goodMagnet + "&dn=" + strings.Repeat("x", 2048), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := types.ValidateMagnet(tc.in, 2048)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, types.ErrInvalidMagnet)
			}
		})
	}
}

func TestValidateChecksum(t *testing.T) {
	good := strings.Repeat("ab", 32)
	require.NoError(t, types.ValidateChecksum(good))
	require.ErrorIs(t, types.ValidateChecksum(strings.ToUpper(good)), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(good[:63]), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(good+"a"), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(""), types.ErrInvalidChecksum)
}

func TestValidateIcon(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte("rest")...)
	svg := []byte("  \n<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	xmlSvg := []byte("<?xml version=\"1.0\"?><svg/>")

	require.NoError(t, types.ValidateIcon(nil, "", 100))
	require.NoError(t, types.ValidateIcon(png, types.MimePNG, 100))
	require.NoError(t, types.ValidateIcon(svg, types.MimeSVG, 100))
	require.NoError(t, types.ValidateIcon(xmlSvg, types.MimeSVG, 100))

	require.ErrorIs(t, types.ValidateIcon(nil, types.MimePNG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, "", 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, types.MimeSVG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(svg, types.MimePNG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, "image/gif", 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, types.MimePNG, 4), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon([]byte{0xff, 0xfe, '<', 's', 'v', 'g'}, types.MimeSVG, 100), types.ErrInvalidIcon)
}

func TestValidateHTTPURL(t *testing.T) {
	require.NoError(t, types.ValidateHTTPURL("", 256))
	require.NoError(t, types.ValidateHTTPURL("https://example.com/x", 256))
	require.NoError(t, types.ValidateHTTPURL("http://example.com", 256))
	require.ErrorIs(t, types.ValidateHTTPURL("ftp://example.com", 256), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateHTTPURL("https:///nohost", 256), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateHTTPURL("https://"+strings.Repeat("a", 256), 256), types.ErrInvalidField)
}

func TestValidateTags(t *testing.T) {
	require.NoError(t, types.ValidateTags(nil, 10, 32))
	require.NoError(t, types.ValidateTags([]string{"wallet", "defi-2"}, 10, 32))
	require.ErrorIs(t, types.ValidateTags([]string{"Wallet"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a b"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{""}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a", "a"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a", "b", "c"}, 2, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{strings.Repeat("a", 33)}, 10, 32), types.ErrInvalidField)
}

func TestValidateText(t *testing.T) {
	require.NoError(t, types.ValidateText("héllo", 10))
	require.ErrorIs(t, types.ValidateText("\xff", 10), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateText("hello world", 5), types.ErrInvalidField)
}

func TestValidateAppMetadata(t *testing.T) {
	p := types.DefaultParams()
	ok := func() error {
		return types.ValidateAppMetadata(p, "Jetty Wallet", "desc", nil, "", "https://x.io", "", "wallet", []string{"a"})
	}
	require.NoError(t, ok())
	require.ErrorIs(t, types.ValidateAppMetadata(p, "", "d", nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, " padded", "d", nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", "d", nil, "", "", "", "nope", nil), types.ErrInvalidCategory)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", strings.Repeat("d", 4097), nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", "d", []byte("x"), "image/gif", "", "", "wallet", nil), types.ErrInvalidIcon)
}
