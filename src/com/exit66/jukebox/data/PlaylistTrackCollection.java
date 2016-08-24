package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.SQLException;

public class PlaylistTrackCollection extends BVDatabaseCollection {

	PlaylistTrack	_currentRecord;
	
	public PlaylistTrackCollection() {
		
		_currentRecord = new PlaylistTrack();
		
	}
		
	public void list(int playlistID) {
		
		if (openConnection()) {
			
			_rs = retrieveData("SELECT * FROM playlist_track WHERE playlist_id = " + playlistID + " ORDER BY play_order");
			try {
				
				if (_rs.next()) {
					_eof = false;
				}
				else {
					_eof = true;
				}
				
			}
			catch (SQLException e) {
			
				_eof = true;
					
			}			
			
		}
		
	}
	
	protected void loadCurrent() {
		
		try {
			
			_currentRecord.setPlaylistID(_rs.getInt("playlist_id"));
			_currentRecord.setTrackID(_rs.getInt("track_id"));
			_currentRecord.setPlayOrder(_rs.getInt("play_order"));
			
		}
		catch (SQLException e) {};
		
	}
	
}
