package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Properties;
import java.io.StringWriter;

import com.exit66.jukebox.Options;

public class Playlist extends BVDatabaseObject {
	
	private int		_playlistID = -1;
	private String	_playlistName;
	
	public int getPlaylistID() {
		
		return _playlistID;
		
	}
	
	public void setPlaylistID(int newValue) {
		
		_playlistID = newValue;
		
	}
	
	public String getPlaylistName() {
		
		return _playlistName;
		
	}
	
	public void setPlaylistName(String newValue) {
		
		_playlistName = newValue;
		
	}
	
	public boolean fetch() {
		
		if (openConnection()) {
		
			ResultSet rs = retrieveData("SELECT * FROM playlist WHERE playlist_id = " + _playlistID);
			
			try {
				
				if (rs.next()) {
					
					_playlistName = rs.getString("playlist_name");
					
				}
				else {
					
					_playlistName = "";
					return false;
					
				}
			
			}	
			catch (SQLException e) {
				
				System.err.println(e);
				
			}
			
			closeConnection();
			
			return true;
			
		}
		else {
			
			return false;
			
		}
	
	}
	
		
	public void save() {
		
		ResultSet rs;
		
		if (openConnection()) {
			
			if (_playlistID == -1) {
		
				this.find();
				
			}
			
			if (_playlistID == -1) {
				
				rs = retrieveData("INSERT INTO playlist (playlist_name) VALUES (" + 
					Qts(_playlistName) + "); CALL IDENTITY()");
					
				try {
					
					if (rs.next()) {
						
						_playlistID = rs.getInt(1);
						
					}
					
				}
				catch (SQLException e) {
					
					System.err.println(e);
					
				}
								
			}
			else {
				
				executeStatement("UPDATE playlist SET playlist_name = " + Qts(_playlistName) + 
					" WHERE playlist_id = " + _playlistID);
				
			}
		
			closeConnection();		
			
		}
		
	}
	
	public void remove() {
		
		if (openConnection()) {
			
			executeStatement("DELETE FROM playlist WHERE playlist_id = " + _playlistID);
			executeStatement("DELETE FROM playlist_track WHERE playlist_id = " + _playlistID);
			
			closeConnection();			
		}
			
	}
		
	public boolean find() {
		
		openConnection();
		
		ResultSet rs = retrieveData("SELECT playlist_id FROM playlist WHERE playlist_name = " + Qts(_playlistName));
				
		try {

			if (rs.next()) {
			
				closeConnection();
				_playlistID = rs.getInt("playlist_id");
				return true;
			
			}
			
		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}
		
		closeConnection();
		return false;
		
	}
	
	public Properties getProperties(Properties prop) {

		prop.setProperty("playlist_id", String.valueOf(_playlistID));
		prop.setProperty("playlist_name", noNull(_playlistName));

		return prop;
				
	}
	
	public void addRule(String ruleType, int ruleValue) {
		addRule(ruleType, Integer.toString(ruleValue));
	}
	
	public void addRule(String ruleType, String ruleValue) {
		openConnection();
		
		ResultSet rs = retrieveData("SELECT COUNT(playlist_rule_id) FROM playlist_rule " + 
					"WHERE playlist_id = " + _playlistID + " AND rule_type = " + 
					Qts(ruleType) + " AND rule_value = " + Qts(ruleValue));
		
		try {
						
			if (rs.next() && rs.getInt(1) == 0) {
				
				executeStatement("INSERT INTO playlist_rule (playlist_id, rule_type, rule_value) " + 
					"VALUES (" + _playlistID + ", " + Qts(ruleType) + ", " + Qts(ruleValue) + ")");
			
			}		
		
		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}
		closeConnection();		
	}
	
	public void removeRule(int playlistRuleID) {
		openConnection();
		
		executeStatement("DELETE FROM playlist_rule WHERE playlist_rule_id = " + playlistRuleID);
		
		closeConnection();		
	}
	
	
	public void removeRules(String rules[]) {
		
		for (int i=0; i<rules.length; i++) {
			
			if (rules[i].length() != 0)
				removeRule(Integer.parseInt(rules[i]));
			
		}
		
	}
	
	public void addTrack(int trackID) {
	
		int nextOrder = 0;
			
		openConnection();
		
		ResultSet rs = retrieveData("SELECT track_id FROM playlist_track WHERE playlist_id = " + _playlistID + " AND track_id = " + trackID);
		
		try {
						
			if (!rs.next()) {
				
				rs = retrieveData("SELECT COUNT(track_id) FROM playlist_track WHERE playlist_id = " + _playlistID + " AND track_id = " + trackID);
				
				rs.next();
			
				if (rs.getInt(1) == 0) {
					
					rs = retrieveData("SELECT MAX(play_order) FROM playlist_track WHERE playlist_id = " + _playlistID);
			
					rs.next();
					
					if (rs.getString(1) != null) {
						
						nextOrder = rs.getInt(1) + 1;
						
					}
				
					executeStatement("INSERT INTO playlist_track (playlist_id, track_id, play_order, playcount) " + 
						"VALUES (" + _playlistID + ", " + trackID + ", " + nextOrder + ", 0)");
				
				}		
			
			}

		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}
		closeConnection();
		
	}
	
	public void removeTrack(int trackID) {
		
		openConnection();
		
		executeStatement("DELETE FROM playlist_track WHERE playlist_id = " + _playlistID + " AND track_id = " + trackID);
		
		closeConnection();
		
	}
	
	public void removeTracks(String tracks[]) {
		
		for (int i=0; i<tracks.length; i++) {
			
			if (tracks[i].length() != 0)
				removeTrack(Integer.parseInt(tracks[i]));
			
		}
		
	}
		
	public void addAlbum(int albumID) {
		
		openConnection();
		
		ResultSet rs = retrieveData("SELECT track_id FROM track WHERE album_id = " + albumID);
		
		try {
			
			while (rs.next()) {
				
				addTrack(rs.getInt("track_id"));
				
			}

		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}		
	}
	
	public void addArtist(int artistID) {
		
		openConnection();
		
		ResultSet rs = retrieveData("SELECT track_id FROM track WHERE artist_id = " + artistID);
		
		try {
			
			while (rs.next()) {
				
				addTrack(rs.getInt("track_id"));
				
			}

		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}		
		
	}
	
	public void empty() {
	
		openConnection();
		
		executeStatement("DELETE FROM playlist_track WHERE playlist_id = " + _playlistID);
		
		closeConnection();
		
	}
	
	public void activate(boolean on) {
		
		openConnection();
					
		if (on) {
		
			executeStatement("UPDATE playlist SET activated = 1 WHERE playlist_id = " + _playlistID);
			
		}
		else {
			
			executeStatement("UPDATE playlist SET activated = 0 WHERE playlist_id = " + _playlistID);
			
		}
		
		closeConnection();
		
	}

	public void deactivateall() {
		
		openConnection();
		
		executeStatement("UPDATE playlist SET activated = 0");
			
		closeConnection();
		
	}
	
	public int getNextTrackID() {
		
		ResultSet rs;
		StringWriter sql = new StringWriter();
				
		BVDatabase conn = new BVDatabase();
		conn.openConnection();
		
		try {
			
			sql.write("SELECT top 1 track_id FROM track WHERE track_id NOT IN " + 
					"(SELECT top " + Options.getPlaylistHistoryCount() + 
					" track_id FROM history ORDER BY entry) " +
					"AND (( track_id IN (SELECT track_id FROM playlist_track " +
					"WHERE playlist_id IN " +
					"(SELECT playlist_id FROM playlist WHERE activated = 1))) ");
			
			rs = conn.retrieveData("SELECT rule_type, rule_value FROM playlist_rule " + 
					"WHERE playlist_id IN (SELECT playlist_id FROM playlist WHERE activated = 1)");
			
			while (rs.next()) {
				sql.write(" OR (" + 
						PlaylistRule.getWhereClause(rs.getString("rule_type"), rs.getString("rule_value")) + ")");
			}
			sql.write(") ORDER BY RAND()");
					
			rs = conn.retrieveData(sql.toString());
				
			if (rs.next()) {
					
				return rs.getInt("track_id");
					
			}
							
		}
		catch (Exception e) {
			
			System.err.println(e);
			
		}
		
		conn.closeConnection();
		
		return -1;
		
	}
	
    public void toJson(StringBuffer sb, int childLevel) {
    	
    }
}
