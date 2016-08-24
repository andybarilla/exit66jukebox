package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.PreparedStatement;
import java.sql.SQLException;

public class TrackCollection extends BVDatabaseCollection {
	
	protected static String TRACK_SELECT = " track.*, album.album_name, album.multiple_artist_id, artist.artist_name, artist.artist_order_name FROM track JOIN album ON track.album_id = album.album_id JOIN artist ON track.artist_id = artist.artist_id ";

	public void list(int start, int count) {

		String sql = "";
		if (openConnection()) {

			if (count == 0)
				sql = "SELECT ";
			else
				sql = "SELECT LIMIT " + start + " " + count;

			_rs = retrieveData(sql + TRACK_SELECT + " ORDER BY track_name");
			try {

				if (_rs.next()) {
					_eof = false;
				} else {
					_eof = true;
				}

			} catch (SQLException e) {

				_eof = true;

			}

		}

	}

	public void list(String search, int start, int count) {

		String sql = "";
		if (openConnection()) {

			try {
				if (count == 0)
					sql = "SELECT ";
				else
					sql = "SELECT LIMIT " + start + " " + count;

				PreparedStatement ps = conn.prepareStatement(sql
						+ TRACK_SELECT + " WHERE track_name LIKE ? "
						+ " OR artist_name LIKE ? OR album_name LIKE ?"
						+ "ORDER BY track_name");
				ps.setString(1, search);
				ps.setString(2, search);
				ps.setString(3, search);
				_rs = retrieveData(ps);

				if (_rs.next()) {
					_eof = false;
				} else {
					_eof = true;
				}

			} catch (SQLException e) {

				_eof = true;

			}

		}

	}

	public void listByArtist(int artistID, int start, int count) {

		String sql = "";
		if (openConnection()) {

			try {
				if (count == 0)
					sql = "SELECT ";
				else
					sql = "SELECT LIMIT " + start + " " + count;

				PreparedStatement ps = conn.prepareStatement(sql
						+ TRACK_SELECT + " WHERE artist_id = ? "
						+ "ORDER BY track_name");
				ps.setInt(1, artistID);
				_rs = retrieveData(ps);

				if (_rs.next()) {
					_eof = false;
				} else {
					_eof = true;
				}

			} catch (SQLException e) {

				_eof = true;

			}

		}
	}
	
	public void listQueue(String sessionID, int start, int count) {

		String sql = "";
		if (openConnection()) {

			try {
				if (count == 0)
					sql = "SELECT ";
				else
					sql = "SELECT LIMIT " + start + " " + count;

				PreparedStatement ps = conn.prepareStatement(sql
						+ TRACK_SELECT + " JOIN request_queue AS rq ON track.track_id = rq.track_id " +
						" WHERE session_id = ? ORDER BY play_order");
				ps.setString(1, sessionID);
				_rs = retrieveData(ps);

				if (_rs.next()) {
					_eof = false;
				} else {
					_eof = true;
				}

			} catch (SQLException e) {

				_eof = true;

			}

		}
	}

	public void listByAlbum(int albumID, int start, int count) {

		String sql = "";
		if (openConnection()) {

			try {
				if (count == 0)
					sql = "SELECT ";
				else
					sql = "SELECT LIMIT " + start + " " + count;

				PreparedStatement ps = conn.prepareStatement(sql
						+ TRACK_SELECT + " WHERE album_id = ? "
						+ "ORDER BY track_number, track_name");
				ps.setInt(1, albumID);
				_rs = retrieveData(ps);

				if (_rs.next()) {
					_eof = false;
				} else {
					_eof = true;
				}

			} catch (SQLException e) {

				_eof = true;

			}

		}
	}

	protected void loadCurrent() {

		try {

			if (_currentRecord == null)
				_currentRecord = new Track();

			((Track) _currentRecord).setTrackID(_rs.getInt("track_id"));
			((Track) _currentRecord).setLibraryID(_rs.getInt("library_id"));
			((Track) _currentRecord).setArtistID(_rs.getInt("artist_id"));
			((Track) _currentRecord).setAlbumID(_rs.getInt("album_id"));
			((Track) _currentRecord).setTrackName(_rs.getString("track_name"));
			((Track) _currentRecord).setFileName(_rs.getString("file_name"));
			((Track) _currentRecord).setContentTypeID(_rs.getInt("content_type_id"));
			((Track) _currentRecord).setTrackNumber(_rs.getInt("track_number"));
			((Track) _currentRecord).setTime(_rs.getString("track_time"));
			((Track) _currentRecord).setPlaycount(_rs.getInt("playcount"));

			Album album = new Album();
			album.setAlbumID(_rs.getInt("album_id"));
			album.setAlbumName(_rs.getString("album_name"));
			album.setMultipleArtistID(_rs.getInt("multiple_artist_id"));
			((Track) _currentRecord).setAlbum(album);

			Artist artist = new Artist();
			artist.setArtistID(_rs.getInt("artist_id"));
			artist.setArtistName(_rs.getString("artist_name"));
			artist.setArtistOrderName(_rs.getString("artist_order_name"));
			((Track) _currentRecord).setArtist(artist);

		} catch (SQLException e) {
			System.err.print(e.toString());
		}
	}

}
