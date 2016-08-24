package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

public class PlaylistTrack extends BVDatabaseObject {

	private int		_playlistID = -1;
	private int		_trackID = -1;
	private int		_playOrder = 0;
	
	public int getPlaylistID() {
		
		return _playlistID;
		
	}
	
	public void setPlaylistID(int newValue) {
		
		_playlistID = newValue;
		
	}
	
	public int getTrackID() {
		
		return _trackID;
		
	}
	
	public void setTrackID(int newValue) {
		
		_trackID = newValue;
		
	}
	
	public int getPlayOrder() {
		
		return _playOrder;
		
	}
	
	public void setPlayOrder(int newValue) {
		
		_playOrder = newValue;
		
	}
	
    public void toJson(StringBuffer sb, int childLevel) {
    	
    }	
}
