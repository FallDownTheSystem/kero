package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"

	"kero/internal/adblock"
	"kero/internal/proto"
)

// adBlock blocks ads and trackers in browser panes (browser-block-ads):
// the first time a pane shows a page with the setting on, it downloads
// the filter lists, if it has to, and compiles them off the main thread,
// once a launch. Pages made before that is done get the rules when it is.
type adBlock struct {
	started bool
	rules   []*mygo.ContentRules // main thread
}

// adRules returns the rules browser panes' pages apply: none while the
// setting is off or the rules are not compiled yet.
func (a *App) adRules() []*mygo.ContentRules {
	if !a.set.BlockAds {
		return nil
	}
	a.startAdBlock()
	return a.ads.rules
}

// startAdBlock compiles the rules, once.
func (a *App) startAdBlock() {
	if a.ads.started {
		return
	}
	a.ads.started = true
	go func() {
		rules, err := compileAdBlock()
		if err != nil {
			log.Printf("kero: ad blocking: %v", err)
		}
		a.update(func() {
			a.ads.rules = rules
			a.applyAdBlock()
		})
	}()
}

// applyAdBlock gives the open browser panes' pages the rules, or takes
// them away, as the setting is; they apply to the loads from then on.
func (a *App) applyAdBlock() {
	rules := a.adRules()
	for _, e := range a.webs {
		if e.view != nil && !e.view.IsDestroyed() {
			e.view.Page().SetContentRules(rules...)
		}
	}
}

// compileAdBlock compiles the filter lists, as many content rule lists as
// they take, keeping those it compiled if one fails.
func compileAdBlock() ([]*mygo.ContentRules, error) {
	filters, err := adblock.Load(context.Background(), filepath.Join(proto.Dir(), "adblock"), adblock.Sources)
	if err != nil {
		return nil, err
	}
	var r adblock.Rules
	for _, f := range filters {
		r.Convert(f)
	}
	var rules []*mygo.ContentRules
	for i, list := range r.Lists() {
		cr, cerr := mygo.CompileContentRules(fmt.Sprintf("kero-ads-%d", i), list)
		if cerr != nil {
			err = cerr
			continue
		}
		rules = append(rules, cr)
	}
	return rules, err
}
