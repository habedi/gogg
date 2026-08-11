package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

func gameWithCover(url string) db.Game {
	return db.Game{ID: 1, Title: "One", CoverImage: url}
}

// The game's own artwork belongs with its screenshots, in the middle of them.
func TestGalleryPicturesFor_PutsTheArtworkInTheMiddle(t *testing.T) {
	pictures, selected := galleryPicturesFor(gameWithCover("//images.gog.com/cover"), shots("http://pics", 4))

	require.Len(t, pictures, 5, "the artwork joins the screenshots")
	require.Equal(t, 2, selected, "the gallery opens on the artwork")
	require.Contains(t, pictures[selected].LargeURL, "cover")
	require.Contains(t, pictures[0].LargeURL, "large")
	require.Contains(t, pictures[4].LargeURL, "large")
}

// A game whose store page has no pictures still has its own artwork.
func TestGalleryPicturesFor_ArtworkOnItsOwn(t *testing.T) {
	pictures, selected := galleryPicturesFor(gameWithCover("//images.gog.com/cover"), nil)

	require.Len(t, pictures, 1)
	require.Equal(t, 0, selected)
	require.Contains(t, pictures[0].LargeURL, "cover")
}

// Catalogues refreshed before gogg recorded artwork fall back to the faded
// background picture, which has to be cropped wherever it is shown.
func TestGalleryPicturesFor_KeepsTrackOfTheFade(t *testing.T) {
	game := db.Game{ID: 1, Title: "One", Data: gameDataWithCover("http://pics/bg.jpg")}
	pictures, _ := galleryPicturesFor(game, nil)

	require.Len(t, pictures, 1)
	require.True(t, pictures[0].Faded, "GOG's background art has a fade baked in")
}

// A game with neither artwork nor screenshots shows nothing at all.
func TestGalleryPicturesFor_NothingToShow(t *testing.T) {
	pictures, selected := galleryPicturesFor(db.Game{ID: 1, Title: "One"}, nil)

	require.Empty(t, pictures)
	require.Equal(t, 0, selected)
}

// Screenshots without artwork start at the first one.
func TestGalleryPicturesFor_ScreenshotsWithoutArtwork(t *testing.T) {
	pictures, selected := galleryPicturesFor(db.Game{ID: 1, Title: "One"}, shots("http://pics", 3))

	require.Len(t, pictures, 3)
	require.Equal(t, 0, selected)
}

// The gallery opens on the artwork and fetches it, not the rest.
func TestGameGallery_OpensOnTheGivenPicture(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	var asked pathLog
	base := storeStub(t, &asked)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 4))
	gallery.show(pictures, selected)

	require.Equal(t, 2, gallery.selected)
	// A thumbnail each, and the artwork again at the size the viewer shows it.
	require.Eventually(t, func() bool { return asked.count.Load() == int64(len(pictures)+1) },
		5*time.Second, 20*time.Millisecond)
	require.True(t, asked.contains("/cover"), "the artwork is what the gallery opens on")
	require.False(t, asked.contains("largea"), "the other pictures stay thumbnails until they are asked for")
}

// The arrow keys move through the pictures, and the ends hold.
func TestGameGallery_ArrowKeysMoveThroughThePictures(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 2))
	gallery.show(pictures, selected)
	require.Equal(t, 1, gallery.selected)

	gallery.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 2, gallery.selected)
	gallery.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 2, gallery.selected, "the last picture holds")

	gallery.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	gallery.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, 0, gallery.selected)
	gallery.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	require.Equal(t, 0, gallery.selected, "the first picture holds")
}

// Clicking a thumbnail shows that picture, and marks which one it is.
func TestGameGallery_TappingAThumbnailShowsIt(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 4))
	gallery.show(pictures, selected)

	test.Tap(gallery.thumbs[4])

	require.Equal(t, 4, gallery.selected)
	require.Equal(t, float32(2), gallery.thumbs[4].border.StrokeWidth, "the picture on show is outlined")
	require.Zero(t, gallery.thumbs[selected].border.StrokeWidth, "and the one before it is not")
}

// A game with one picture has nothing to choose between, so it gets no strip.
func TestGameGallery_HidesTheStripForASinglePicture(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	gallery.show(galleryPicturesFor(gameWithCover(base+"/cover"), nil))
	require.False(t, gallery.scroll.Visible())

	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 2))
	gallery.show(pictures, selected)
	require.True(t, gallery.scroll.Visible())
}

// Nothing to show leaves the space to the rest of the pane.
func TestGameGallery_HidesItselfWithNoPictures(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	gallery.show(nil, 0)

	require.False(t, gallery.viewer.Visible())
	require.False(t, gallery.scroll.Visible())
}

// A picture fetched for the game that was on screen a moment ago must not be
// shown for the one that is there now.
func TestGameGallery_DropsPicturesForThePreviousGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	// The pictures are held back until both games have been shown, so what
	// arrives is answering the gallery as it was, not as it is.
	base, release, dir := heldPictureServer(t)

	gallery := newGameGallery(testCoverCache(t, dir), test.NewWindow(nil))
	gallery.show([]galleryPicture{{ThumbnailURL: base + "/one", LargeURL: base + "/one"}}, 0)
	stale := gallery.thumbs[0]

	gallery.show([]galleryPicture{{ThumbnailURL: base + "/two", LargeURL: base + "/two"}}, 0)

	close(release)
	require.Eventually(t, func() bool { return cachedFiles(dir) == 2 }, 5*time.Second, 20*time.Millisecond)
	require.Nil(t, stale.picture.Resource, "the thumbnail of the previous game stays empty")
}

// Selecting a game puts its artwork in the gallery, and the screenshots join it
// when GOG answers.
func TestLibraryTab_SelectingAGameShowsItsPictures(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		game := db.Game{ID: 1, Title: "Game 1", Data: richGameData, CoverImage: base + "/cover"}
		require.NoError(t, lt.selected.Set(game))

		require.Len(t, lt.gallery.pictures, 1, "the artwork shows before GOG has been asked")
		require.True(t, lt.gallery.viewer.Visible())

		showGallery(lt.gallery, game, shots(base, 4))
		require.Len(t, lt.gallery.pictures, 5)
		require.Equal(t, 2, lt.gallery.selected, "the artwork stays what is shown")
	})
}

// Selecting nothing empties the gallery rather than leaving the last game's
// pictures on screen.
func TestLibraryTab_GalleryEmptiesWhenNothingIsSelected(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	offMain(t, func() {
		lt, _ := newLibraryFixture(t, 2)
		require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", CoverImage: base + "/cover"}))
		require.NotEmpty(t, lt.gallery.pictures)

		require.NoError(t, lt.selected.Set(nil))
		require.Empty(t, lt.gallery.pictures)
	})
}

// heldPictureServer answers only once the returned channel is closed, so a test
// can decide when a picture comes back. It also owns the cache directory, which
// must outlive the fetches writing into it.
func heldPictureServer(t *testing.T) (string, chan struct{}, string) {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write(onePixelPNG)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, release, t.TempDir()
}

// The artwork takes the width the pane has to spare rather than sitting at one
// size with gaps beside it, and stops growing before it pushes the facts off.
func TestGameGallery_ArtworkFollowsThePaneWidth(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 3))
	gallery.show(pictures, selected)

	win := test.NewWindow(gallery)
	t.Cleanup(win.Close)

	win.Resize(fyne.NewSize(460, 900))
	narrow := gallery.viewer.picture.MinSize()

	win.Resize(fyne.NewSize(900, 900))
	wide := gallery.viewer.picture.MinSize()

	require.Greater(t, wide.Width, narrow.Width, "a wider pane shows larger artwork")
	require.LessOrEqual(t, wide.Height, float32(maxViewerHeight), "but only up to a point")
	require.InDelta(t, 16.0/9.0, float64(wide.Width/wide.Height), 0.05, "and it keeps its shape")
}

// The arrow keys move through the pictures, so the gallery has to show when it
// is the thing they will move.
func TestGameGallery_ShowsWhenItHasTheKeyboard(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	gallery := newGameGallery(nil, nil)
	gallery.show(shotPictures(2), 0)
	require.Zero(t, gallery.viewer.border.StrokeWidth, "nothing has the keyboard yet")

	gallery.FocusGained()
	require.Positive(t, gallery.viewer.border.StrokeWidth, "the gallery has to say it has the keyboard")

	gallery.FocusLost()
	require.Zero(t, gallery.viewer.border.StrokeWidth, "and say when it does not")
}

// shotPictures is a gallery of plain screenshots.
func shotPictures(n int) []galleryPicture {
	pictures := make([]galleryPicture, 0, n)
	for i := 0; i < n; i++ {
		pictures = append(pictures, galleryPicture{
			ThumbnailURL: fmt.Sprintf("http://pics/thumb%d", i),
			LargeURL:     fmt.Sprintf("http://pics/large%d", i),
		})
	}
	return pictures
}

// The picture viewer's backdrop follows the theme rather than freezing at
// whatever it was when the widget was built: in the dark theme it is dark,
// not the white it used to show.
func TestGalleryImage_BackdropFollowsTheTheme(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	dark := theme.VariantDark
	a.Settings().SetTheme(&GoggTheme{Theme: theme.DefaultTheme(), variant: &dark})

	img := newGalleryImage(fyne.NewSize(100, 100), nil)
	img.Refresh()

	r, g, b, _ := img.border.FillColor.RGBA()
	require.Less(t, r>>8, uint32(0x40), "the backdrop is dark in the dark theme")
	require.Less(t, g>>8, uint32(0x40))
	require.Less(t, b>>8, uint32(0x40))
	require.Equal(t, theme.Color(theme.ColorNameBackground), img.border.FillColor)
}

// The gallery takes the keyboard, so it has to answer the whole focusable
// contract, not only the arrow keys it acts on.
func TestGameGallery_AnswersTheFocusableContract(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 3))
	gallery.show(pictures, selected)

	gallery.FocusGained()
	require.True(t, gallery.viewer.border.StrokeWidth > 0, "the focused gallery outlines what the keys will move")
	gallery.FocusLost()
	require.Zero(t, gallery.viewer.border.StrokeWidth)

	require.False(t, gallery.AcceptsTab(), "tab moves on rather than into the gallery")

	// A typed rune is not one of the gallery's keys and must change nothing.
	before := gallery.selected
	gallery.TypedRune('x')
	require.Equal(t, before, gallery.selected)

	// Tapping asks for the keyboard. There is no canvas behind the widget in
	// a test, so this only has to leave the gallery as it was.
	gallery.Tapped(nil)
	require.Equal(t, before, gallery.selected)
}

// Opening a picture is what a tap on the viewer does. With no window there is
// nothing to open into, and nothing must go wrong either.
func TestGameGallery_OpenSelectedNeedsAWindowAndAPicture(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	base := storeStub(t, nil)

	windowless := newGameGallery(testCoverCache(t, t.TempDir()), nil)
	windowless.show(galleryPicturesFor(gameWithCover(base+"/cover"), nil))
	windowless.openSelected()

	gallery := newGameGallery(testCoverCache(t, t.TempDir()), test.NewWindow(nil))
	gallery.openSelected() // nothing has been shown yet

	pictures, selected := galleryPicturesFor(gameWithCover(base+"/cover"), shots(base, 2))
	gallery.show(pictures, selected)
	gallery.openSelected()
	require.Equal(t, selected, gallery.selected, "opening a picture leaves the gallery on it")
}
