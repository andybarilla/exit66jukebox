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

public class PlaylistCollection extends BVDatabaseCollection {

	Playlist 		_currentRecord;
	
	public PlaylistCollection() {
		
		_currentRecord = new Playlist();
		
	}
	
	public void listActive() {
	
		processList("WHERE activated = 1");
			
	}
	
	public void listInactive() {
	
		processList("WHERE activated = 0 OR activated is null");	
	}
		
	public void list() {
		
		processList("");	
		
	}
	
	private void processList(String whereClause) {
		
		if (openConnection()) {
			
			_rs = retrieveData("SELECT * FROM playlist " + whereClause + " ORDER BY playlist_name");
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
			_currentRecord.setPlaylistName(_rs.getString("playlist_name"));
			
		}
		catch (SQLException e) {};
		
	}

}
