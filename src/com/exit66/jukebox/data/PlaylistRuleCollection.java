package com.exit66.jukebox.data;

import java.sql.SQLException;

public class PlaylistRuleCollection extends BVDatabaseCollection {

	PlaylistRule	_currentRecord;
	
	public PlaylistRuleCollection() {
		
		_currentRecord = new PlaylistRule();
		
	}
		
	public void list(int playlistID) {
		
		if (openConnection()) {
			
			_rs = retrieveData("SELECT * FROM playlist_rule WHERE playlist_id = " + playlistID);
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
			
			_currentRecord.setPlaylistRuleID(_rs.getInt("playlist_rule_id"));
			_currentRecord.setPlaylistID(_rs.getInt("playlist_id"));
			_currentRecord.setRuleType(_rs.getString("rule_type"));
			_currentRecord.setRuleValue(_rs.getString("rule_value"));
			
		}
		catch (SQLException e) {};
		
	}
}
