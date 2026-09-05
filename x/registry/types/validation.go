package types

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/semver/v3"

	errorsmod "cosmossdk.io/errors"
)

var (
	btihRe     = regexp.MustCompile(`^urn:btih:([0-9a-fA-F]{40}|[A-Za-z2-7]{32})$`)
	checksumRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tagRe      = regexp.MustCompile(`^[a-z0-9-]+$`)
	pngSig     = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
)

// ValidateSemver enforces strict MAJOR.MINOR.PATCH[-prerelease], no build metadata, no leading v (SPEC §6.4).
func ValidateSemver(s string, maxBytes uint32) error {
	if len(s) == 0 || len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidSemver, "length %d not in 1..%d", len(s), maxBytes)
	}
	v, err := semver.StrictNewVersion(s)
	if err != nil {
		return errorsmod.Wrap(ErrInvalidSemver, err.Error())
	}
	if v.Metadata() != "" {
		return errorsmod.Wrap(ErrInvalidSemver, "build metadata is not allowed")
	}
	return nil
}

// ValidateMagnet requires scheme magnet and at least one xt=urn:btih:<40 hex | 32 base32>.
func ValidateMagnet(s string, maxBytes uint32) error {
	if len(s) == 0 || len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidMagnet, "length %d not in 1..%d", len(s), maxBytes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return errorsmod.Wrap(ErrInvalidMagnet, err.Error())
	}
	if u.Scheme != "magnet" {
		return errorsmod.Wrapf(ErrInvalidMagnet, "scheme %q is not magnet", u.Scheme)
	}
	for _, xt := range u.Query()["xt"] {
		if btihRe.MatchString(xt) {
			return nil
		}
	}
	return errorsmod.Wrap(ErrInvalidMagnet, "no urn:btih exact topic")
}

// ValidateChecksum requires exactly 64 lowercase hex characters.
func ValidateChecksum(s string) error {
	if !checksumRe.MatchString(s) {
		return errorsmod.Wrap(ErrInvalidChecksum, "must be 64 lowercase hex characters")
	}
	return nil
}

// ValidateIcon checks size, mime, and file signature (SPEC §6.4).
func ValidateIcon(icon []byte, mime string, maxBytes uint32) error {
	if len(icon) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidIcon, "%d bytes > max %d", len(icon), maxBytes)
	}
	if len(icon) == 0 {
		if mime != "" {
			return errorsmod.Wrap(ErrInvalidIcon, "icon_mime must be empty when icon is empty")
		}
		return nil
	}
	switch mime {
	case MimePNG:
		if !bytes.HasPrefix(icon, pngSig) {
			return errorsmod.Wrap(ErrInvalidIcon, "missing PNG signature")
		}
	case MimeSVG:
		if !utf8.Valid(icon) {
			return errorsmod.Wrap(ErrInvalidIcon, "svg is not valid UTF-8")
		}
		trimmed := bytes.TrimLeft(icon, " \t\r\n")
		if !bytes.HasPrefix(trimmed, []byte("<svg")) && !bytes.HasPrefix(trimmed, []byte("<?xml")) {
			return errorsmod.Wrap(ErrInvalidIcon, "svg must start with <svg or <?xml")
		}
	default:
		return errorsmod.Wrapf(ErrInvalidIcon, "unsupported icon_mime %q", mime)
	}
	return nil
}

// ValidateHTTPURL allows empty; otherwise http(s) with a host and bounded length.
func ValidateHTTPURL(s string, maxBytes uint32) error {
	if s == "" {
		return nil
	}
	if len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidField, "url: %d bytes > max %d", len(s), maxBytes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidField, "url: %s", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errorsmod.Wrap(ErrInvalidField, "url must be http(s) with a host")
	}
	return nil
}

// ValidateTags enforces count, length, charset [a-z0-9-], and uniqueness.
func ValidateTags(tags []string, maxTags, maxTagBytes uint32) error {
	if len(tags) > int(maxTags) {
		return errorsmod.Wrapf(ErrInvalidField, "tags: %d > max %d", len(tags), maxTags)
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if len(tag) == 0 || len(tag) > int(maxTagBytes) {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: length not in 1..%d", tag, maxTagBytes)
		}
		if !tagRe.MatchString(tag) {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: must match [a-z0-9-]+", tag)
		}
		if _, dup := seen[tag]; dup {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: duplicate", tag)
		}
		seen[tag] = struct{}{}
	}
	return nil
}

// ValidateText requires valid UTF-8 within maxBytes.
func ValidateText(s string, maxBytes uint32) error {
	if !utf8.ValidString(s) {
		return errorsmod.Wrap(ErrInvalidField, "text is not valid UTF-8")
	}
	if len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidField, "text: %d bytes > max %d", len(s), maxBytes)
	}
	return nil
}

// ValidateAppMetadata applies every app-level rule from SPEC §6.2 using the given params.
func ValidateAppMetadata(p Params, title, description string, icon []byte, iconMime, website, sourceURL, category string, tags []string) error {
	if err := ValidateText(title, p.MaxTitleBytes); err != nil {
		return errorsmod.Wrap(err, "title")
	}
	if len(title) == 0 || strings.TrimSpace(title) != title {
		return errorsmod.Wrap(ErrInvalidField, "title must be non-empty without leading/trailing whitespace")
	}
	if err := ValidateText(description, p.MaxDescriptionBytes); err != nil {
		return errorsmod.Wrap(err, "description")
	}
	if err := ValidateIcon(icon, iconMime, p.MaxIconBytes); err != nil {
		return err
	}
	if err := ValidateHTTPURL(website, p.MaxWebsiteBytes); err != nil {
		return errorsmod.Wrap(err, "website")
	}
	if err := ValidateHTTPURL(sourceURL, p.MaxSourceUrlBytes); err != nil {
		return errorsmod.Wrap(err, "source_url")
	}
	found := false
	for _, c := range p.Categories {
		if c == category {
			found = true
			break
		}
	}
	if !found {
		return errorsmod.Wrapf(ErrInvalidCategory, "%q", category)
	}
	return ValidateTags(tags, p.MaxTags, p.MaxTagBytes)
}
