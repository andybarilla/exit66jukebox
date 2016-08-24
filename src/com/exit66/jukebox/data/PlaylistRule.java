package com.exit66.jukebox.data;

public class PlaylistRule extends BVDatabaseObject {

	private int		_playlistRuleID = -1;
	private int		_playlistID = -1;
	private String	_ruleType = "";
	private String	_ruleValue = "";
	
	public int getPlaylistID() {
		
		return _playlistID;
		
	}
	
	public void setPlaylistID(int newValue) {
		
		_playlistID = newValue;
		
	}
	
	public int getPlaylistRuleID() {
		
		return _playlistRuleID;
		
	}
	
	public void setPlaylistRuleID(int newValue) {
		
		_playlistRuleID = newValue;
		
	}
	
	public String getRuleType() {
		
		return _ruleType;
		
	}
		public void setRuleType(String newValue) {
		
		_ruleType = newValue;
		
	}
	
	public String getRuleValue() {
		
		return _ruleValue;
		
	}
	
	public void setRuleValue(String newValue) {
		
		_ruleValue = newValue;
		
	}
	
	public String getRuleDisplayValue(String ruleType, String ruleValue) {
		try {
			if (ruleType.equals("library")) {
				Library lib = new Library();
				lib.setLibraryID(Integer.parseInt(ruleValue));
				if (lib.fetch()) 
					return lib.getLibraryPath();			
			}
			else if (ruleType.equals("artist")) {
				Artist artist = new Artist();
				artist.setArtistID(Integer.parseInt(ruleValue));
				if (artist.fetch()) 
					return artist.getArtistName();			
			}
			else if (ruleType.equals("genre")) {
				ContentType genre = new ContentType();
				genre.setContentTypeID(Integer.parseInt(ruleValue));
				if (genre.fetch()) 
					return genre.getContentType();			
			}
		}
		catch (Exception e) {
			System.err.println(e);
		}
		return ruleValue;
	}

	public static String getWhereClause(String ruleType, String ruleValue) {
		if (ruleType.equals("library")) {
			return "library_id = " + ruleValue;			
		}
		else if (ruleType.equals("artist")) {
			return "artist_id = " + ruleValue;			
		}
		else if (ruleType.equals("genre")) {
			return "content_type_id = " + ruleValue;			
		}
		return "";
	}
	
    public void toJson(StringBuffer sb, int childLevel) {
    	
    }
}
