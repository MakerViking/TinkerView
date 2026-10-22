package camera

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/MakerViking/TinkerView/internal/devices"
)

// EditRequest is a camera edit. Every field is optional: blank NewName/NewSrc*
// keep the current value, and a blank Username/Password keeps the stored one
// (they are applied independently). ClearCredentials removes both and wins over
// any username/password sent with it.
type EditRequest struct {
	NewName          string
	NewSrcKind       string
	NewSrcInput      string
	Username         string
	Password         string
	ClearCredentials bool
}

// Info is the client-facing description of a camera for the edit form. The
// password is never included (HasPassword only) and SrcInput has any URL
// credentials stripped.
type Info struct {
	Name         string `json:"name"`
	SrcKind      string `json:"srcKind"`
	SrcInput     string `json:"srcInput"`
	Username     string `json:"username"`
	HasPassword  bool   `json:"hasPassword"`
	Managed      bool   `json:"managed"`
	Promoted     bool   `json:"promoted"`
	ConfigStream bool   `json:"configStream"`
	// Editable: the source/credentials can be edited here (UI-managed cameras and
	// Frigate-config go2rtc streams). The name is only editable when Managed &&
	// !Promoted.
	Editable bool `json:"editable"`
	Rotate   int  `json:"rotate"`
	FlipH    bool `json:"flipH"`
	FlipV    bool `json:"flipV"`
}

// Edit changes a camera's name, source and/or credentials. It returns async=true
// when the change was queued as a Frigate-restarting background job (a promoted
// camera's source/credentials; progress/outcome come via notices).
//
// A rename needs a non-promoted, UI-managed camera. The source of a camera that
// lives in the Frigate config's go2rtc.streams (not UI-managed) is rewritten in
// that config and applied live to go2rtc, with no Frigate restart; its name stays
// read-only.
func (m *Manager) Edit(name string, req EditRequest) (async bool, err error) {
	newName := strings.TrimSpace(req.NewName)
	renaming := newName != "" && newName != name
	if renaming {
		if err := devices.ValidateCameraName(newName); err != nil {
			return false, err
		}
	}

	var asyncWork func(context.Context)
	err = func() error {
		ctx, cancel := context.WithTimeout(m.base, opTimeout)
		defer cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		rec, ok := m.store.Camera(name)
		if !ok || rec.ConfigStream {
			if renaming {
				return fmt.Errorf("camera %q is defined in the Frigate config; its name can't be changed here", name)
			}
			var cur *devices.CameraRecord
			if ok {
				cur = &rec
			}
			return m.editConfigStreamLocked(ctx, name, cur, req)
		}

		newRec := rec
		curKind := rec.SrcKind
		if curKind == "" {
			curKind = devices.InferSrcKind(rec.Src)
		}
		credsChanged, err := applyCredentials(&newRec, req)
		if err != nil {
			return err
		}

		// Source: a new URL, and/or a new kind (which re-wraps the current URL).
		kind := normKind(req.NewSrcKind)
		if strings.TrimSpace(req.NewSrcKind) == "" {
			kind = curKind
		}
		input := strings.TrimSpace(req.NewSrcInput)
		if input == "" && kind != curKind {
			input = devices.SrcInputOf(curKind, rec.Src)
		}
		srcChanged := false
		if input != "" {
			newSrc, err := m.resolveSrc(kind, input)
			if err != nil {
				return err
			}
			// The edit form shows the URL with credentials stripped; when the
			// stored URL embeds them (legacy) and none are supplied, keep them.
			if !req.ClearCredentials && rec.Username == "" {
				if _, ui, _ := devices.SplitUserinfo(rec.Src); ui != "" {
					if _, nui, _ := devices.SplitUserinfo(newSrc); nui == "" {
						newSrc = devices.WithUserinfo(newSrc, ui)
					}
				}
			}
			newRec.SrcKind = kind // also backfills legacy records that lack it
			if newSrc != rec.Src {
				newRec.Src, srcChanged = newSrc, true
			}
		}
		if renaming {
			newRec.Name = newName
		}

		// Promoted cameras are owned by Frigate: rename needs a demote, but a
		// source/credential change is re-applied to Frigate asynchronously.
		if rec.Promoted {
			if renaming {
				return fmt.Errorf("camera %q is promoted to Frigate; demote it before renaming", name)
			}
			if !srcChanged && !credsChanged {
				return nil
			}
			asyncWork = func(ctx context.Context) {
				if e := m.editPromotedSource(ctx, newRec); e != nil {
					m.notice("error", e.Error())
				}
			}
			return nil
		}

		// Non-promoted: apply synchronously. Store first (rolled back if go2rtc
		// then rejects the stream), so a failed edit leaves store == live.
		if renaming {
			if _, exists := m.store.Camera(newName); exists {
				return fmt.Errorf("a camera named %q already exists", newName)
			}
			if err := m.checkStreamCollision(ctx, newName); err != nil {
				return err
			}
			if _, err := m.store.RenameCamera(name, newRec); err != nil {
				return fmt.Errorf("save camera: %w", err)
			}
			if err := m.g2.PutStream(ctx, newName, m.sourceWithTransform(newRec)); err != nil {
				if _, rerr := m.store.RenameCamera(newName, rec); rerr != nil {
					slog.Error("camera rename rollback failed", "from", newName, "to", name, "err", rerr)
				}
				_ = m.g2.DeleteStream(ctx, newName)
				return fmt.Errorf("go2rtc rejected the renamed stream (nothing changed): %w", err)
			}
			if err := m.g2.DeleteStream(ctx, name); err != nil {
				slog.Warn("camera rename: old go2rtc stream not removed", "name", name, "err", err)
			}
			m.publishDeleted(name)
			m.publishManaged(newRec)
		} else {
			if !srcChanged && !credsChanged {
				return nil
			}
			if err := m.store.UpsertCamera(newRec); err != nil {
				return fmt.Errorf("save camera: %w", err)
			}
			if err := m.g2.PutStream(ctx, name, m.sourceWithTransform(newRec)); err != nil { // PUT replaces the source
				if rerr := m.store.UpsertCamera(rec); rerr != nil {
					slog.Error("camera edit rollback failed", "name", name, "err", rerr)
				}
				return fmt.Errorf("go2rtc rejected the stream (nothing changed): %w", err)
			}
			m.publishManaged(newRec)
		}
		slog.Info("camera edited", "name", newRec.Name)
		return nil
	}()
	if err != nil {
		return false, err
	}
	if asyncWork != nil {
		m.async(asyncWork)
		return true, nil
	}
	return false, nil
}

// applyCredentials applies the request's credential changes to rec and reports
// whether anything changed. Username and password are independent: each is only
// replaced when non-blank. ClearCredentials removes both (and any credentials
// embedded in the source URL) and overrides the other fields.
func applyCredentials(rec *devices.CameraRecord, req EditRequest) (bool, error) {
	if req.ClearCredentials {
		stripped := devices.StripUserinfo(rec.Src)
		changed := rec.Username != "" || rec.Password != "" || stripped != rec.Src
		rec.Username, rec.Password, rec.Src = "", "", stripped
		return changed, nil
	}
	changed := false
	if u := strings.TrimSpace(req.Username); u != "" && u != rec.Username {
		rec.Username, changed = u, true
	}
	if req.Password != "" && req.Password != rec.Password {
		rec.Password, changed = req.Password, true
	}
	if rec.Username == "" && rec.Password != "" {
		return false, fmt.Errorf("a password needs a username")
	}
	return changed, nil
}

// Info describes a camera for the edit form (nil-safe on unknown names). It
// covers UI-managed cameras from the store and, when Frigate config editing is
// available, hand-authored cameras restreamed through go2rtc.streams.
func (m *Manager) Info(ctx context.Context, name string) (Info, bool) {
	if rec, ok := m.store.Camera(name); ok {
		kind := rec.SrcKind
		if kind == "" {
			kind = devices.InferSrcKind(rec.Src)
		}
		user, hasPass := rec.Username, rec.Password != ""
		if user == "" {
			_, ui, _ := devices.SplitUserinfo(rec.Src)
			user, hasPass = splitUserPass(ui)
		}
		return Info{
			Name: rec.Name, SrcKind: kind, SrcInput: devices.SrcInputOf(kind, rec.Src),
			Username: user, HasPassword: hasPass,
			Managed: !rec.ConfigStream, Promoted: rec.Promoted, ConfigStream: rec.ConfigStream,
			Editable: true, Rotate: rec.Rotate, FlipH: rec.FlipH, FlipV: rec.FlipV,
		}, true
	}
	if m.frigate == nil || strings.HasSuffix(name, "_hq") {
		return Info{}, false
	}
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return Info{}, false
	}
	base, isCam, err := readConfigStream(raw, name)
	if err != nil {
		return Info{}, false
	}
	kind := devices.InferSrcKind(base)
	_, ui, _ := devices.SplitUserinfo(base)
	user, hasPass := splitUserPass(ui)
	return Info{
		Name: name, SrcKind: kind, SrcInput: devices.SrcInputOf(kind, base),
		Username: user, HasPassword: hasPass,
		Promoted: isCam, ConfigStream: true, Editable: true,
	}, true
}

// splitUserPass splits "user[:pass]" userinfo.
func splitUserPass(ui string) (user string, hasPass bool) {
	if i := strings.IndexByte(ui, ':'); i >= 0 {
		return ui[:i], ui[i+1:] != ""
	}
	return ui, false
}

// editConfigStreamLocked edits the source/credentials of a camera whose go2rtc
// stream lives in the Frigate config: rec is its adopted store record (image
// transform) or nil for a plain hand-authored camera. The stream entry is
// rewritten in the Frigate config (backup + validate/save WITHOUT restart) and
// the new source is patched into the running go2rtc, so no restart is needed.
// *_hq exec streams are never touched. Caller holds m.mu.
func (m *Manager) editConfigStreamLocked(ctx context.Context, name string, rec *devices.CameraRecord, req EditRequest) error {
	if m.frigate == nil {
		return fmt.Errorf("unknown camera %q (Frigate config editing is not available)", name)
	}
	if strings.HasSuffix(name, "_hq") {
		return fmt.Errorf("%q is a generated GPU-enhanced stream and can't be edited", name)
	}
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	cfgSrc, _, err := readConfigStream(raw, name)
	if err != nil {
		return err
	}
	// The pristine base source: the adopted record's, else what the config holds.
	base := cfgSrc
	if rec != nil {
		base = rec.Src
	}
	curKind := devices.InferSrcKind(base)
	if rec != nil && rec.SrcKind != "" {
		curKind = rec.SrcKind
	}
	kind := normKind(req.NewSrcKind)
	if strings.TrimSpace(req.NewSrcKind) == "" {
		kind = curKind
	}

	newBase := base
	input := strings.TrimSpace(req.NewSrcInput)
	if input == "" && kind != curKind {
		input = devices.SrcInputOf(curKind, base)
	}
	if input != "" {
		newBase, err = m.resolveSrc(kind, input)
		if err != nil {
			return err
		}
		// Credentials stripped from the form stay with the stream unless cleared.
		if _, ui, _ := devices.SplitUserinfo(base); ui != "" && !req.ClearCredentials {
			if _, nui, _ := devices.SplitUserinfo(newBase); nui == "" {
				newBase = devices.WithUserinfo(newBase, ui)
			}
		}
	}
	// Credentials live in the URL for these streams (the Frigate config has no
	// separate fields). Existing parts (possibly {ENV} placeholders) are kept
	// verbatim; new parts are escaped.
	if req.ClearCredentials {
		newBase = devices.StripUserinfo(newBase)
	} else if u, p := strings.TrimSpace(req.Username), req.Password; u != "" || p != "" {
		if !strings.Contains(newBase, "://") {
			return fmt.Errorf("this stream's source has no URL to carry credentials")
		}
		_, ui, _ := devices.SplitUserinfo(newBase)
		eu, ep, _ := strings.Cut(ui, ":")
		if u != "" {
			eu = escapeUserinfo(u)
		}
		if p != "" {
			ep = escapeUserinfo(p)
		}
		if eu == "" {
			return fmt.Errorf("a password needs a username")
		}
		nui := eu
		if ep != "" {
			nui += ":" + ep
		}
		newBase = devices.WithUserinfo(newBase, nui)
	}
	if newBase == base {
		return nil // nothing to change
	}

	next := devices.CameraRecord{Name: name, Src: newBase}
	if rec != nil {
		next = *rec
		next.Src = newBase
	}
	next.SrcKind = kind
	live := newBase
	if rec != nil {
		var terr error
		if live, terr = devices.BuildTransformSource(next.Source(), next.Rotate, next.FlipH, next.FlipV, m.hardware); terr != nil {
			return terr
		}
	}

	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := updateStreamSourceYAML(raw, name, live)
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	// saveonly validates AND writes the config without restarting Frigate.
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err)
	}
	if rec != nil {
		if err := m.store.UpsertCamera(next); err != nil {
			if rerr := m.frigate.Validate(ctx, raw); rerr != nil {
				slog.Error("camera edit: could not restore Frigate config", "backup", backupPath, "err", rerr)
			}
			return fmt.Errorf("save camera: %w", err)
		}
		m.publishManaged(next)
	}

	// Apply live. {ENV} placeholders in the credentials are expanded by Frigate at
	// start-up only, so a source carrying them can't be patched into go2rtc.
	if _, ui, _ := devices.SplitUserinfo(live); strings.ContainsAny(ui, "{}") {
		m.notice("info", fmt.Sprintf("Camera %q saved to the Frigate config; it keeps {ENV} credentials, so the change applies on the next Frigate restart", name))
		return nil
	}
	if err := m.g2.PatchStream(ctx, name, live); err != nil {
		return fmt.Errorf("saved to the Frigate config (backup: %s) but go2rtc rejected the live update, so it will apply on the next Frigate restart: %w", backupPath, err)
	}
	slog.Info("config-stream camera source edited", "name", name)
	return nil
}

// escapeUserinfo percent-encodes one part of URL userinfo (user or password).
func escapeUserinfo(s string) string {
	return strings.TrimSuffix(url.UserPassword(s, "").String(), ":")
}
