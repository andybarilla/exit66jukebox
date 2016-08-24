// create and return a span as a link and assign a callback to the click event
function getLink(text, callback, class_name, id) {
    "use strict";

    var div = $("<div>")
            .addClass("link")
            .text(text)
            .click(callback);
    if (id !== undefined) {
        div.attr("id", id);
    }
    if (class_name !== undefined) {
        div.addClass(class_name);
    }
    return div;
}

// return a div that serves as the divider between buttons on the header
function getHeaderBar() {
    "use strict";

    return $("<div>").addClass("header-bar");
}

// same as getHeaderBar but floats right instead of left
function getHeaderBarRight() {
    "use strict";

    return $("<div>").addClass("header-bar-right");
}

// return a div with the break class
function getBreakDiv() {
    "use strict";

    return $("<div>").addClass("break");
}

// show a message - just an alert for now
function showAlert(message) {
    "use strict";

	if ($.x.alert.data("timeout") !== undefined) {
		clearTimeout($.x.alert.data("timeout"));
		$.x.alert.removeData("timeout");
	}
	$.x.alert.text(message);
	$.x.alert.css("display", "block");
	
	$.x.alert.data("timeout", setTimeout(hideAlert, 3000));
}

// timeout function to hide alert
function hideAlert() {
	"use strict";
	
	$.x.alert.removeData("timeout")
	$.x.alert.fadeOut('fast', function() {
		$.x.alert.css("display", "none");
	});
}

// show the current playing song
function showCurrentSong(track) {
    "use strict";

    if ($("#now-playing").data("nowplaying") === undefined) {
        $("#now-playing").text("");
        $("#now-playing").append("Now playing: ",
                $("<span>").addClass("song-name"),
                " by ",
                $("<span>").addClass("artist-name"));
        $("#now-playing").data("nowplaying", true);
    }
    $("#now-playing .song-name").text(track.name);
    $("#now-playing .artist-name").text(track.artist_name);
}

// clear the now playing area
function showNothingPlaying() {
    $("#now-playing").removeData("nowplaying");
    $("#now-playing").children().remove();
    $("#now-playing").text("Please request a song");
}

// play a file 
function playFile(id) {
    "use strict";

    if ($.x.usingHtml5Audio === true) {
        $.x.player.attr("src", "./rest/track/" + id + "?action=stream");

        $.x.player.get(0).play();
    } else {
        niftyplayer('niftyPlayer1').loadAndPlay("./rest/track/" + id +
                "?action=stream");
    }
}

// pause/play the player
function pausePlayer() {
	"use strict";
	
    if ($.x.usingHtml5Audio === true) {
        if ($.x.player.data("state") === "paused") {
            $.x.player.get(0).play();
        } else if ($.x.player.data("state") === "playing") {
            $.x.player.get(0).pause();
        }
    } else {
        niftyplayer('niftyPlayer1').playToggle();
    }
}

// check if there's another file in the queue to play
// if not, go to sleep and check again later
function nextFile() {
    "use strict";
		
    $.ajax({
        type: "get",
        url: "./rest/track/queue",
        dataType: "json",
        success: function (response) {
            if (response.success === 1) {
                playFile(response.results.track_id);
                showCurrentSong(response.results);
            }
            else {
                showNothingPlaying();
                $.x.player.attr("src", "");
                setTimeout(nextFile, 1000);
            }
        }
    });
}

// configure the audio player
function setupPlayer() {
    "use strict";

    try {
        var audio = $("<audio>");
        var audio_el = audio.get(0);

        if ((false) && (audio_el.canPlayType !== undefined) && 
            (audio_el.canPlayType("audio/mpeg"))) {
            $.x.usingHtml5Audio = true;
            audio
                .bind("ended", nextFile)
                .bind("pause", function () {
                    $.x.player.data("state", "paused")
                })
                .bind("playing", function () { 
                    $.x.player.data("state", "playing") 
                })
                .data("state", "stopped");
            $.x.player = audio;
            $("#header").append($.x.player);
            nextFile();
        } else {
            $.x.usingHtml5Audio = false;
            audio = $("<div>")
                .css({
                    position: "absolute",
                    top: "-200px"
                });
            $.x.player = audio;
            $("#header").append($.x.player);
            audio.flash({
                    swf: 'niftyplayer.swf',
                    id: 'niftyPlayer1',
                    width: 165,
                    height:38
                });
            setupFlashPlayer();
        }
    }
    catch (err) {
        setTimeout(setupPlayer, 500);
    }
}

function setupFlashPlayer() {
    try {
        niftyplayer('niftyPlayer1')
            .registerEvent('onSongOver', 'nextFile()');
        nextFile();
    }
    catch (err) {
        setTimeout(setupFlashPlayer, 500);
    }
}


// request a song
function requestSong(id) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/track/" + id,
        data: { action: 'request' },
        dataType: "json",
        success: function (response) {
			showAlert(response.message);
        }
    });
}

// request an album
function requestAlbum(id) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/album/" + id,
        data: { action: "request" },
        dataType: "json",
        success: function (response) {
            showAlert(response.message);
        }
    });
}

// remove a song from the queue
function removeSongFromQueue(id) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/queue/" + id,
        data: { action: "remove" },
        dataType: "json",
        success: function (response) {
            if (response.success !== 1) {
                showAlert(response.message);
            }
        }
    });
}

// clear the whole queue
function clearQueue(onsuccess) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/queue", 
        data: { action: "clear" },
        dataType: "json",
        success: function (response) {
            onsuccess();
        }
    });
}

// check if the number of songs has changed in the library
function hasLibraryChanged(section, trueFunction, falseFunction) {
	"use strict";
	
	$.ajax({
        type: "get",
        url: "./rest/stats",
        dataType: "json",
        success: function (response) {
			var lastCount;
			if ($.x.lastStats !== undefined) {
				if ($.x.lastStats[section] !== undefined) {
					lastCount = $.x.lastStats[section].Songs;
				} else {
					lastCount = 0;
				}
			} else {
				$.x.lastStats = {};
				lastCount = 0;
			}
			$.x.lastStats[section] = response.results;
			
            if (response.results.Songs !== lastCount) {
				if (trueFunction !== undefined) {
					trueFunction();
				}
			} else {
				if (falseFunction !== undefined) {
					falseFunction();
				}
            }
        }
    });	
}

// switch the main area to show a new screen
function showDivInMain(div, rememberScrollTop, oncomplete) {
    "use strict"; 

    if ($.x.currentDiv !== undefined) {
        if ($.x.currentDiv.data("button") !== undefined) {
            $.x.currentDiv.data("button").removeClass("selected-button");
        }
        if ($.x.currentDiv.attr("temp") === "1") {
            $.x.currentDiv.remove()
        }
        else {
            if (rememberScrollTop === true) {
                $.x.currentDiv.attr("lastscrolltop", 
                        $.x.currentDiv.scrollTop());
            }
            $.x.currentDiv.detach();
        }
    }
    $.x.main.append(div);
    if (div.attr("lastscrolltop") !== undefined) {
        div.scrollTop(div.attr("lastscrolltop"));
        div.removeAttr("lastscrolltop");
    }
    $.x.currentDiv = div;
    if ($.x.currentDiv.data("button") !== undefined) {
        $.x.currentDiv.data("button").addClass("selected-button");
    }
    if (div.refresh !== undefined) {
        div.refresh();
    }
	if (oncomplete !== undefined) {
		oncomplete();
	}
}

// create the subdiv area which will be displayed in the main area
function createSubDiv(divclass) {
    "use strict"; 

    return $("<div>")
        .addClass(divclass)
        .addClass("subdiv");
}

// fill a table with songs
//   if onclick is not specified request a song will be the funcionality
//   if artist_id is specified the artist names will only show if they don't
//       match this id
//   if include_album is true then the album will also be displayed
//   if include_number is true then the track number will be displayed
function addToTableOfSongs(table, tracks, args) {
    "use strict"

    if (args === undefined) {
        args = {};
    }

    if (args.onclick === undefined) {
        args.onclick = function (track, tr) { 
            requestSong(track.track_id);
        };
    }

    if (args.include_album === undefined) {
        args.include_album = false;
    }

    if (args.include_number === undefined) {
        args.include_number = true;
    }

    $.each(
        tracks,
        function (index, value) {
            var songartist = " ";
            var tr = $("<tr>");
            if (value.artist_id !== args.artist_id) {
                songartist = value.artist_name;
            }
            if (args.include_number) {
                tr.append($("<td>")
                    .addClass("number")
                    .text(value.number));
            }
            tr.append(
                $("<td>").text(value.name).addClass("song"),
                $("<td>").text(songartist).addClass("artist"));
            if (args.include_album) {
                tr.append($("<td>").text(value.album_name).addClass("album"));
            }
            tr.click(function () {
                args.onclick(value, tr);
            });
            table.append(tr);
        }
    );
}

// returns the standard table that is used for songs
function getTableOfSongs(album, args) {
    "use strict"; 

    var table = $("<table>")
        .addClass("songs");

    if (album !== undefined) {
        addToTableOfSongs(table, album.tracks, args);
    }
    return table;
}

// adds the header to a songs table
function addHeaderToTableOfSongs(table, args) {
    if (args === undefined) {
        args = {};
    }

    if (args.include_album === undefined) {
        args.include_album = false;
    }

    if (args.include_number === undefined) {
        args.include_number = true;
    }
	
	if (args.song_header === undefined) {
		args.song_header = "Song";
	}

    var tr = $("<tr>");
    if (args.include_number === true) {
        tr.append($("<th>").text("#"));
    }
    tr.append($("<th>").text(args.song_header),
              $("<th>").text("Artist"));
    if (args.include_album === true) {
        tr.append($("<th>").text("Album"));
    }
    table.append($("<thead>").append(tr));
}

// return a div containing an album details
function getAlbumDetailDiv(album) {
    return $("<div>").addClass("album")
        .attr('temp', '1')
        .append($("<div>")
                .addClass("album-side")
                .append($("<div>")
                    .addClass("details")
                    .append($("<img>")
                        .attr(
                            {"src": "/rest/album/" + album.album_id + 
                                 "?action=image&size=150",
                             "width": "150px",
                             "height": "150px"}))
                    .append($("<div>")
                        .text(album.name)
                        .addClass("album-name"))
                    .append($("<div>")
                        .text(album.multiple_artist_name)
                        .addClass("artist-name")))
                .append($("<div>")
                    .text("Play entire album")
                    .addClass("request-album")
                    .click(function () {
                        requestAlbum(album.album_id);
                    })))
        .append($("<div>")
                    .addClass("album-songs")
                    .append(getTableOfSongs(album, { "artist_id": album.multiple_artist_id })),
                getBreakDiv());
}

// display the list of albums for an artist on the artists tab
function displayArtistsAlbums(artist) {
    "use strict"; 

    var albumsdiv = $("<div>")
        .addClass("artist-albums")
        .addClass("subdiv");

    albumsdiv.append(getLink("back",
            function () {
                showDivInMain($.x.artistsdiv);
            }, "back-button"));

    $.each(
        artist.albums,
        function (index, value) {
            albumsdiv.append(getAlbumDetailDiv(value)); 
        }
    );    
    showDivInMain(albumsdiv, true);
}

// load the list of albums for an artist on the artists tab
function loadArtistsAlbums(artist_id) {
    "use strict"; 

    $.ajax({
        type: "get",
        url: "./rest/artist/" + artist_id + "?track_detail=1",
        dataType: "json",
        success: function (response) {
            displayArtistsAlbums(response.results);
        }
    });
}

// add artist items to a ul
function appendArtistsToList(list, items) {
    "use strict"; 

    var htmlBuffer = [];
    var li;
    var imagediv;

    $.each(
        items,
        function (index, value) {
            htmlBuffer.push("<li class=\"artist-item\"\
                onclick=\"loadArtistsAlbums(" + value.artist_id + ");\">");
            htmlBuffer.push("<div><div class=\"artist-image\"><img src=\"/rest/artist/" +
                value.artist_id + "?action=image&size=150,100\" /></div>");
            htmlBuffer.push("<div class=\"artist-name\">" +
                value.name + "</div>");
            htmlBuffer.push("<div class=\"counts\">" + 
                value.album_count + ' albums, ' + value.track_count +
                ' songs</div>');
            htmlBuffer.push("</li>");
        }
    );  

    list.append(htmlBuffer.join(''));
}

// load artists list from the database
function loadMoreArtists(list, onComplete) {
    "use strict"; 

    if (list.data("xhr")) {
        // we're already waiting for data to exit
        return;
    }
    var nextOffset = (list.data("nextOffset") || 0);
    var limit = 30; 
    var resultsCount = 0;
    
    list.data(
        "xhr",
        $.ajax({
            type: "get",
            url: "./rest/artist",
            data: {
                limit: limit,
                offset: nextOffset
            },
            dataType: "json",
            success: function (response) {
                resultsCount = response.results.length;
                appendArtistsToList(list, response.results);
            },
            complete: function () {
                list.removeData("xhr");
                if (resultsCount !== 0) {
                    list.data("nextOffset", nextOffset + limit);
                    onComplete();
                }
            } 
        })
    );        
}

// checks if more artists should be loaded based off
// the scrolling location
function isMoreArtistsNeeded(scrollContainer, listContainer) {
    "use strict"; 

    var viewTop = scrollContainer.scrollTop();
    var viewBottom = viewTop + scrollContainer.height();
    var containerBottom = Math.floor(scrollContainer.offset().top +
            listContainer.height());
    var scrollBuffer = 300; // twice the height of an artist li

    if ((containerBottom - scrollBuffer) <= viewBottom) {
        return true;
    } else {
        return false; 
    }
}

// check to see if more items should be loaded in the artists list
// and if so load them
function checkArtistsContents(container, list) {
    "use strict"; 

    if (isMoreArtistsNeeded($.x.artistsdiv, container)) {
        loadMoreArtists(list,
                function () {
                    checkArtistsContents(container, list)
                });
    }
}

// switch to the artist browsing screen
function showArtists() {
    "use strict"; 
	
    // if we"ve never loaded the artists area before create it
    if ($.x.artistsdiv === undefined) {
        var container = $("<div>");
        var list = $("<ul>").attr("id", "artists-list");
        container.append(list, getBreakDiv());
                
        $.x.artistsdiv = createSubDiv("artists");
        $.x.artistsdiv.data("button", $("#header-artists"));
        
        $.x.artistsdiv.bind(
                "scroll resize",
                function (event) {
                    checkArtistsContents(container, list);
                });

        $.x.artistsdiv.append(container);
        checkArtistsContents(container, list);
    } else {
		hasLibraryChanged("artists", function () {
			var list = $("#artists-list");
			list.data("nextOffset", 0);
			list.children().remove();
			$.x.artistsdiv.trigger("scroll");
		});
	}

    showDivInMain($.x.artistsdiv);

}

// display the details of an album on the albums tab
function displayAlbumDetails(album) {
    "use strict"; 

    var albumsdiv = $("<div>")
        .addClass("albums-detail")
        .addClass("subdiv");

    albumsdiv.append(getLink("back",
            function () {
                showDivInMain($.x.artistsdiv);
            }, "back-button"));

    albumsdiv.append(getAlbumDetailDiv(album)); 
    showDivInMain(albumsdiv, true);
}

// load the details of an album on the albums tab
function loadAlbumDetails(album_id) {
    "use strict"; 

    $.ajax({
        type: "get",
        url: "./rest/album/" + album_id,
        dataType: "json",
        success: function (response) {
            displayAlbumDetails(response.results);
        }
    });
}

// add album items to a ul
function appendAlbumsToList(list, items) {
    "use strict"; 

    var htmlBuffer = [];
    var li;
    var imagediv;

    $.each(
        items,
        function (index, value) {
            htmlBuffer.push("<li class=\"artist-item\"\
                onclick=\"loadAlbumDetails(" + value.album_id + ");\">");
            htmlBuffer.push("<div><div class=\"album-image\"><img src=\"/rest/album/" +
                value.album_id + "?action=image&size=150\" /></div>");
            htmlBuffer.push("<div class=\"album-name\">" +
                value.name + "</div>");
            htmlBuffer.push("<div class=\"artist-name\">" +
                value.multiple_artist_name + "</div>");
            htmlBuffer.push("<div class=\"counts\">" + 
                value.track_count + ' songs</div>');
            htmlBuffer.push("</li>");
        }
    );  

    list.append(htmlBuffer.join(''));
}

// load artists list from the database
function loadMoreAlbums(list, onComplete) {
    "use strict"; 

    if (list.data("xhr")) {
        // we're already waiting for data to exit
        return;
    }
    var nextOffset = (list.data("nextOffset") || 0);
    var limit = 30; 
    var resultsCount = 0;
    
    list.data(
        "xhr",
        $.ajax({
            type: "get",
            url: "./rest/album",
            data: {
                limit: limit,
                offset: nextOffset
            },
            dataType: "json",
            success: function (response) {
                resultsCount = response.results.length;
                appendAlbumsToList(list, response.results);
            },
            complete: function () {
                list.removeData("xhr");
                if (resultsCount !== 0) {
                    list.data("nextOffset", nextOffset + limit);
                    onComplete();
                }
            } 
        })
    );        
}

// checks if more albums should be loaded based off
// the scrolling location
function isMoreAlbumsNeeded(scrollContainer, listContainer) {
    "use strict"; 

    var viewTop = scrollContainer.scrollTop();
    var viewBottom = viewTop + scrollContainer.height();
    var containerBottom = Math.floor(scrollContainer.offset().top +
            listContainer.height());
    var scrollBuffer = 300; // twice the height of an artist li

    if ((containerBottom - scrollBuffer) <= viewBottom) {
        return true;
    } else {
        return false; 
    }
}

// check to see if more items should be loaded in the artists list
// and if so load them
function checkAlbumsContents(container, list) {
    "use strict"; 

    if (isMoreAlbumsNeeded($.x.albumsdiv, container)) {
        loadMoreAlbums(list,
                function () {
                    checkAlbumsContents(container, list)
                });
    }
}

// switch to the album browsing screen
function showAlbums() {
    "use strict"; 

    // if we've never loaded the albums area before create it
    if ($.x.albumsdiv === undefined) {
        var container = $("<div>");
        var list = $("<ul>").attr("id", "albums-list");
        container.append(list, getBreakDiv());
                
        $.x.albumsdiv = createSubDiv("albums");
        $.x.albumsdiv.data("button", $("#header-albums"));

        $.x.albumsdiv.bind(
                "scroll resize",
                function (event) {
                    checkAlbumsContents(container, list);
                });

        $.x.albumsdiv.append(container);
        checkAlbumsContents(container, list);
    } else {
		hasLibraryChanged("albums", function () {
			var list = $("#albums-list");
			list.data("nextOffset", 0);
			list.children().remove();
			$.x.albumsdiv.trigger("scroll");
		});
	}

    showDivInMain($.x.albumsdiv);
}

// load songs list from the database
function loadMoreSongs(table, onComplete) {
    "use strict"; 

    if (table.data("xhr")) {
        // we're already waiting for data to exit
        return;
    }
    var nextOffset = (table.data("nextOffset") || 0);
    var limit = 30; 
    var resultsCount = 0;
    
    table.data(
        "xhr",
        $.ajax({
            type: "get",
            url: "./rest/track",
            data: {
                limit: limit,
                offset: nextOffset
            },
            dataType: "json",
            success: function (response) {
                resultsCount = response.results.length;
                addToTableOfSongs(table, response.results, 
                    { include_album: true,
                      include_number: false });
            },
            complete: function () {
                table.removeData("xhr");
                if (resultsCount !== 0) {
                    table.data("nextOffset", nextOffset + limit);
                    onComplete();
                }
            } 
        })
    );        
}

// checks if more songs should be loaded based off
// the scrolling location
function isMoreSongsNeeded(scrollContainer, tableContainer) {
    "use strict"; 

    var viewTop = scrollContainer.scrollTop();
    var viewBottom = viewTop + scrollContainer.height();
    var containerBottom = Math.floor(scrollContainer.offset().top +
            tableContainer.height());
    var scrollBuffer = 300; 

    if ((containerBottom - scrollBuffer) <= viewBottom) {
        return true;
    } else {
        return false; 
    }
}

// check to see if more items should be loaded in the artists list
// and if so load them
function checkSongsContents(container, table) {
    "use strict"; 

    if (isMoreSongsNeeded($.x.songsdiv, container)) {
        loadMoreSongs(table,
                function () {
                    checkSongsContents(container, table)
                });
    }
}

// switch to the songs browsing screen
function showSongs() {
    "use strict";

    // if we've never loaded the songs area before create it
    if ($.x.songsdiv === undefined) {
        var container = $("<div>").addClass("songs-list");
        var table = getTableOfSongs();
        addHeaderToTableOfSongs(table,
                { include_album: true, include_number: false});
        container.append(table, getBreakDiv());
                
        $.x.songsdiv = createSubDiv("songs");
        $.x.songsdiv.data("button", $("#header-songs"));

        $.x.songsdiv.bind(
                "scroll resize",
                function (event) {
                    checkSongsContents(container, table);
                });

        $.x.songsdiv.append(container);
        checkSongsContents(container, table);
    } else {
		hasLibraryChanged("songs", function () {
			var table = $(".songs-list").children(".songs")
			table.find("tr:gt(0)").remove();
			table.data("nextOffset", 0);
			$.x.songsdiv.trigger("scroll");
		});
	}

    showDivInMain($.x.songsdiv);
}

// load songs list from the database
function loadMoreQueue(table, onComplete) {
    "use strict"; 

    if (table.data("xhr")) {
        // we're already waiting for data to exit
        return;
    }
    var nextOffset = (table.data("nextOffset") || 0);
    var limit = 30; 
    var resultsCount = 0;
	
	var click_function = function (track, tr) {
        var tbody = tr.parent();
        var admin_row = $("<tr>")
            .append($("<td>")
                    .append($("<button>")
                        .text("Remove")
                        .click(function () {
                            removeSongFromQueue(track.track_id);
                            tr.remove();
                            admin_row.remove();
                            tbody.removeData("admin-row");
                        })));
        if (tbody.data("admin-row") !== undefined) {
            tbody.data("admin-row").remove();
        }
        tbody.data("admin-row", admin_row);
        tr.after(admin_row);
    };
    
    table.data(
        "xhr",
        $.ajax({
            type: "get",
            url: "./rest/queue",
            data: {
                limit: limit,
                offset: nextOffset
            },
            dataType: "json",
            success: function (response) {
				resultsCount = response.results.length;
                if ((resultsCount === 0) && (nextOffset === 0)) {
					table.append($("<tr>")
						.append($("<td>")
							.text("No songs requested. Please request something")
							.attr("colspan", "3")));
				} else {
					addToTableOfSongs(table, response.results, 
						{ onclick: click_function, 
						  include_album: true,
						  include_number: false });
				}
            },
            complete: function () {
                table.removeData("xhr");
                if (resultsCount !== 0) {
                    table.data("nextOffset", nextOffset + limit);
                    onComplete();
                }
            } 
        })
    );        
}

// checks if more songs should be loaded based off
// the scrolling location
function isMoreQueueNeeded(scrollContainer, tableContainer) {
    "use strict"; 

    var viewTop = scrollContainer.scrollTop();
    var viewBottom = viewTop + scrollContainer.height();
    var containerBottom = Math.floor(scrollContainer.offset().top +
            tableContainer.height());
    var scrollBuffer = 300; 

    if ((containerBottom - scrollBuffer) <= viewBottom) {
        return true;
    } else {
        return false; 
    }
}

// check to see if more items should be loaded in the artists list
// and if so load them
function checkQueueContents(container, table) {
    "use strict"; 

    if (isMoreQueueNeeded($.x.queuediv, container)) {
        loadMoreQueue(table,
                function () {
                    checkQueueContents(container, table)
                });
    }
}

// switch to the songs browsing screen
function showQueue() {
    "use strict";

    // if we've never loaded the songs area before create it
    if ($.x.queuediv === undefined) {
        var container = $("<div>").addClass("songs-list");
        var table = getTableOfSongs();
        addHeaderToTableOfSongs(table,
                { song_header: "Upcoming Songs", include_album: true, include_number: false});
        container.append(table, getBreakDiv());
                
        $.x.queuediv = createSubDiv("queue");
        $.x.queuediv.data("button", $("#header-queue"));

        $.x.queuediv.bind(
                "scroll resize",
                function (event) {
                    checkQueueContents(container, table);
                });
				
				
        $.x.queuediv.append(getLink("Clear queue", 
                    function () {
                        clearQueue(function () {
							table.find("tr:gt(0)").remove();
							table.removeData("nextOffset");
                            checkQueueContents(container, table);
                        })
                    }, "clear-queue"));
		$.x.queuediv.append(getBreakDiv());

        $.x.queuediv.append(container);
		
        checkQueueContents(container, table);
		
		$.x.queuediv.refresh = function () {
			table.find("tr:gt(0)").remove();
			table.removeData("nextOffset");
            checkQueueContents(container, table);
        };
    }

    showDivInMain($.x.queuediv);
}

// load song search list from the database
function loadMoreSearchResults(table, search, onComplete) {
    "use strict"; 

    if (table.data("xhr")) {
        // we're already waiting for data to exit
        return;
    }
    var nextOffset = (table.data("nextOffset") || 0);
    var limit = 30; 
    var resultsCount = 0;
    
    table.data(
        "xhr",
        $.ajax({
            type: "get",
            url: "./rest/track",
            data: {
                search: search,
                limit: limit,
                offset: nextOffset
            },
            dataType: "json",
            success: function (response) {
                resultsCount = response.results.length;
                addToTableOfSongs(table, response.results, 
                    { include_album: true,
                      include_number: false });
            },
            complete: function () {
                table.removeData("xhr");
                if (resultsCount !== 0) {
                    table.data("nextOffset", nextOffset + limit);
                    onComplete();
                }
            } 
        })
    );        
}

// checks if more search results should be loaded based off
// the scrolling location
function isMoreSearchResultsNeeded(scrollContainer, tableContainer) {
    "use strict"; 

    var viewTop = scrollContainer.scrollTop();
    var viewBottom = viewTop + scrollContainer.height();
    var containerBottom = Math.floor(scrollContainer.offset().top +
            tableContainer.height());
    var scrollBuffer = 300; 

    if ((containerBottom - scrollBuffer) <= viewBottom) {
        return true;
    } else {
        return false; 
    }
}

// check to see if more items should be loaded in the artists list
// and if so load them
function checkSearchResultsContents(container, table, search) {
    "use strict"; 

    if (isMoreSearchResultsNeeded($.x.searchdiv, container)) {
        loadMoreSearchResults(table,
                search,
                function () {
                    checkSearchResultsContents(container, table, search)
                });
    }
}

// switch to the search screen
function showSearch() {
    "use strict";

	var search_input;
	var search_form;
	var container;
	var table;
	
    // if we've never loaded the search area before create it
    if ($.x.searchdiv === undefined) {
        container = $("<div>").addClass("songs-list");
        table = getTableOfSongs();
        addHeaderToTableOfSongs(table,
                { include_album: true, include_number: false});
        search_input = $("<input>").addClass("search-field");
        search_form = $("<form>")
            .addClass("search-form")
            .append(search_input,
                    $("<input>")
						.attr("type", "submit")
                        .text("Search"))
			.submit(function () {  
				table.find("tr:gt(0)").remove();
				table.removeData("nextOffset");
				checkSearchResultsContents(container, table,
					search_input.val());
				$.x.searchdiv.unbind("scroll resize");
				$.x.searchdiv.bind(
						"scroll resize",
						function (event) {
							checkSearchResultsContents(container,
								table, search_input.val());
						});
				return false;
			});
                    
        container.append(table, getBreakDiv());
                
        $.x.searchdiv = createSubDiv("search");
        $.x.searchdiv.data("button", $("#header-search"));

        $.x.searchdiv.append(search_form);
        $.x.searchdiv.append(container);
    }

    showDivInMain($.x.searchdiv, null, function () {
		$(".search-field").focus();
	});	
}

// remove a library from the database
function deleteLibrary(library) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/library/" + library.library_id,
        data: {
            action: "delete"
        },
        dataType: "json",
        success: function (response) {
            if (response.success === 1) {
                showAlert('Library deleted');
                $("tr[admin-library-id=\"" + library.library_id + "\"]").remove();
            } else {
                showAlert(response.message);
            }
        }
    });
}

// scan a library in the database
function scanLibrary(library) {
    "use strict";

    $.ajax({
        type: "post",
        url: "./rest/library/" + library.library_id,
        data: {
            action: "scan"
        },
        dataType: "json",
        success: function (response) {
            if (response.success === 1) {
                showAlert('Library scanning started');
            } else {
                showAlert(response.message);
            }
        }
    });
}

// adds a library to the library table if it's not already there
function addLibraryToList(table, library) {
    "use strict";

	var scan_link;
	if (library.scan_status === 0) {
		scan_link = $("<a>")
			.text("scan")
			.attr("href", "")
			.click(function () {
				scanLibrary(library);
				$.x.admindiv.refresh();
				return false;
			});
	} else {
		scan_link = "Scanning...";
	}
	
    if ($("tr[admin-library-id=\"" + library.library_id + "\"]").length === 0) {
        var del_link = $("<a>")
            .text("delete")
            .attr("href", "")
            .click(function () {
                deleteLibrary(library);
                return false;
            });

        table.append($("<tr>")
                .append($("<td>").text(library.path),
                        $("<td>").append(del_link),
						$("<td>").append(scan_link))
                .attr("admin-library-id", library.library_id));
    } else {
		var td = $("tr[admin-library-id=\"" + library.library_id + "\"]").find("td:last");
		td.children.remove();
		td.append(scan_link);
	}
}

// loads the library list
//    if refresh is true it will clear the table first 
function loadLibraryList(table, refresh) {
    "use strict";

    if (refresh === undefined) {
        refresh = true;
    }
    $.ajax({
        type: "get",
        url: "./rest/library",
        dataType: "json",
        success: function (response) {
            if (refresh === true) {
                table.find("tr:gt(0)").remove();
            }
            for (var i=0; i<response.results.length; i++) {
                addLibraryToList(table, response.results[i]);
            }
        }
    });
}

// loads the stats list
function loadStatsList(table) {
    "use strict";

    $.ajax({
        type: "get",
        url: "./rest/stats",
        dataType: "json",
        success: function (response) {
            table.find("tr:gt(0)").remove();
            for (var i in response.results) {
				table.append($("<tr>")
					.append($("<td>").text(i),
							$("<td>").text(response.results[i])));
            }
        }
    });
}

// add a library to the database and add it to a table
function addLibrary(table, path) {
    "use strict";
    
    $.ajax({
        type: "post",
        url: "./rest/library",
        data: {
            action: "add",
            path: path.val()
        },
        dataType: "json",
        success: function (response) {
            if (response.success === 1) {
                showAlert('Library added');
                loadLibraryList(table);
                path.val("");
            } else {
                showAlert(response.message);
            }
        }
    });
}

// switch to the admin screen
function showAdmin() {
    "use strict";
    
    // if we've never loaded the search area before create it
    if ($.x.admindiv === undefined) {
        var lib_table = $("<table>")
            .addClass("library")
            .append($("<thead>")
                    .append($("<tr>")
                        .append($("<th>").text("Library Path"),
                                $("<th>").text(""),
								$("<th>").text(""))));
        var new_lib_input = $("<input>");
        var new_lib = $("<div>")
			.addClass("new-library-form")
            .append("New Library: ", new_lib_input,
                    $("<button>")
                        .text("Add")
                        .click(function () {
                            addLibrary(lib_table, new_lib_input);
                        }));

		var stats_table = $("<table>")
            .addClass("stats")
            .append($("<thead>")
                    .append($("<tr>")
                        .append($("<th>").text("Statistics"),
                                $("<th>").text(""))));
	
		$.x.admindiv = createSubDiv("admin");
        $.x.admindiv.data("button", $("#header-admin"));

		$.x.admindiv.append($("<h1>").text("Global Options"));
        $.x.admindiv.append($("<div>")
                    .addClass("library-list")
					.append(lib_table, new_lib));
        //$.x.admindiv.append();
		$.x.admindiv.append($("<div>")
					.addClass("stats-list")
					.append(stats_table));

        $.x.admindiv.refresh = function () {
            loadLibraryList(lib_table);
			loadStatsList(stats_table);
        };
    }

    showDivInMain($.x.admindiv);
}

// setup header
function setupHeader() {
    "use strict";
    
    var player = $("<div>")
        .addClass("player") 
        .append(getLink("Play/Pause", pausePlayer),
                getLink("Next Song", nextFile));

    var now_playing = $("<div>")
        .attr("id", "now-playing");

    $.x.header.append(
            getLink("artists", showArtists, "header-button", "header-artists"),
            getHeaderBar(),
            getLink("albums", showAlbums, "header-button", "header-albums"),
            getHeaderBar(),
            getLink("songs", showSongs, "header-button", "header-songs"),
            getHeaderBar(),
            getLink("search", showSearch, "header-button", "header-search"),
            getHeaderBar(),
            getLink("", showAdmin, "admin-button", "header-admin"),
            getHeaderBarRight(),
            now_playing,
			getLink("queue", showQueue, "header-button header-queue", "header-queue"),getHeaderBarRight());
}

// setup the basic interface
function setupInterface() {
    "use strict";

    // create the main two areas
    $.x.header = $("<div>")
        .attr("id", "header");
    $.x.main = $("<div>")
        .attr("id", "main");
	$.x.alert = $("<div>")
		.attr("id", "alert")
		.click(hideAlert);
		
    
    // fill in the header
    setupHeader();

    // add the main areas to the body
    $("body").append($.x.header, $.x.main, $.x.alert);
}

$(document).ready(function() {
    "use strict";

    // setup app associative array
    $.x = {};

    // setup the main interface
    setupInterface();

    // setup audio player
    setupPlayer();

	// check if there are any songs in the database 
	// if not, send to settings screen instead of browsing
	$.ajax({
        type: "get",
        url: "./rest/stats",
        dataType: "json",
        success: function (response) {
            if (response.results.Songs === 0) {
				showAdmin();
			} else {
				showArtists();
            }
        }
    });
});
