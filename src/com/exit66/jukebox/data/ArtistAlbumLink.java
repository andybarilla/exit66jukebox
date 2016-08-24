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

public class ArtistAlbumLink extends BVDatabase {
	
	private int         _artistID;
	private int         _albumID;
        
	public int getArtistID() {
		
		return _artistID;
		
	}
	
	public void setArtistID(int newValue) {
		
		_artistID = newValue;
		
	}
	
	public int getAlbumID() {
		
		return _albumID;
		
	}
	
	public void setAlbumID(int newValue) {
		
		_albumID = newValue;
		
	}
        
	public void save() {
				
		if (openConnection()) {
			
			if (!this.find()) {
		
				executeStatement("INSERT INTO artistalbumlink (artist_id, album_id) VALUES (" + 
					_artistID + ", " + _albumID + ")");
								
			}

			closeConnection();		
			
		}
		
	}
	
	public void remove() {
		
		if (openConnection()) {
			
			executeStatement("DELETE FROM artistalbumlink WHERE artist_id = " + _artistID +
				" AND album_id = " + _albumID);
			
			closeConnection();			
			
		}
			
	}
		
	public boolean find() {
	
		openConnection();
				
		ResultSet rs = retrieveData("SELECT album_id FROM artistalbumlink WHERE artist_id = " + _artistID + 
			" AND album_id = " + _albumID);
				
		try {

			if (rs != null && rs.next()) {

				closeConnection();			
				return true;
			
			}
			
		}
		catch (SQLException e) {
			
			System.err.println(e);
			
		}
		
		closeConnection();
		return false;
		
	}

}
