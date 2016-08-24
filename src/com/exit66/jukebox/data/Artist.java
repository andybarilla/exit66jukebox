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
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Iterator;
import java.util.Map;

import com.exit66.jukebox.Options;

public class Artist extends BVDatabaseObject {
    
    private int 	_artistID = -1;
    private String	_artistName;
    private String	_artistOrderName;
    private int 	_albumCount = 0;
    private int 	_trackCount = 0;
    
    public int getArtistID() {
        
        return _artistID;
        
    }
    
    public void setArtistID(int newValue) {
        
        _artistID = newValue;
        
    }
    
    public String getArtistName() {
        
        return _artistName;
        
    }
    
    public void setArtistName(String newValue) {
        
        if (newValue != null)
            _artistName = newValue.trim();
        else
            _artistName = null;
        
    }
    
    public String getArtistOrderName() {
        
        return _artistOrderName;
        
    }
    
    public void setArtistOrderName(String newValue) {
        
        if (newValue != null)
            _artistOrderName = newValue.trim();
        else
            _artistOrderName = null;
        
    }
    
    public int getAlbumCount() {
    	return _albumCount;
    }
    
    public void setAlbumCount(int value) {
    	_albumCount = value;
    }
    
    public int getTrackCount() {
    	return _trackCount;
    }
    
    public void setTrackCount(int value) {
    	_trackCount = value;
    }
    
    public boolean fetch() {
        
        if (openConnection()) {
            
            try {
            
            	PreparedStatement ps = conn.prepareStatement("SELECT * FROM artist WHERE artist_id = ?");
            	ps.setInt(1, _artistID);
            	ResultSet rs = retrieveData(ps);                
                
                if (rs != null && rs.next()) {
                    
                    _artistName = rs.getString("artist_name");
                    _artistOrderName = rs.getString("artist_order_name");
                    _albumCount = rs.getInt("album_count");
                    _trackCount = rs.getInt("track_count");
                    
                } else {
                    
                    _artistName = "";
                    _artistOrderName = "";
                    _albumCount = 0;
                    _trackCount = 0;
                    return false;
                    
                }
                
            } catch (SQLException e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
            return true;
            
        } else {
            
            return false;
            
        }
        
    }
    
    public void save() {
        
        ResultSet rs;
        
        if ((_artistOrderName != null) && (_artistOrderName.length() == 0)) {
        	guessArtistOrderName();
        }
        
        if (openConnection()) {
            
            try {
                
                if (_artistID == -1) {
                    
                    this.find();
                    
                }
                
                if (_artistID == -1) {
                    
                	try {
                    
                    	conn.setAutoCommit(false);
                    	PreparedStatement ps = conn.prepareStatement("INSERT INTO artist (artist_name, artist_order_name, " +
                    			"album_count, track_count) VALUES (?, ?, ?, ?)");
                    	ps.setString(1, _artistName);
                    	ps.setString(2, _artistOrderName);
                    	ps.setInt(3, _albumCount);
                    	ps.setInt(4, _trackCount);
                    	executeStatement(ps);
                    	
                    	rs = retrieveData("CALL IDENTITY()");
                    	
                        if (rs.next()) {
                            
                            _artistID = rs.getInt(1);
                            
                        }
                        conn.commit();
                        conn.setAutoCommit(true);
                        
                    } catch (SQLException e) {
                        
                        System.err.println(e);
                        
                    }
                    
                } else {
                    
                	PreparedStatement ps = conn.prepareStatement("UPDATE artist SET artist_name = ?, " +
                            "artist_order_name = ?, album_count = ?, track_count = ? WHERE artist_id = ?");
                	ps.setString(1, _artistName);
                	ps.setString(2, _artistOrderName);
                	ps.setInt(3, _albumCount);
                	ps.setInt(4, _trackCount);
                	ps.setInt(5, _artistID);
                	executeStatement(ps);
                    
                }
                
            } catch (Exception e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
        }
        
    }
        
    public boolean find() {
        
        openConnection();
        
        if ((_artistOrderName != null) && (_artistOrderName.length() == 0)) {
            guessArtistOrderName();
        }
        
        try {
        
        	PreparedStatement ps = conn.prepareStatement("SELECT artist_id FROM artist " + 
                "WHERE artist_name = ? OR artist_order_name = ?");
        	ps.setString(1, _artistName);
        	ps.setString(2, _artistOrderName);
        	ResultSet rs = retrieveData(ps);
            
            if (rs != null && rs.next()) {
                
                closeConnection();
                _artistID = rs.getInt("artist_id");
                return true;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return false;
        
    }
    
    public void guessArtistOrderName() {
    	guessArtistOrderName(Options.getReplace());    	
    }
        
    public void guessArtistOrderName(Map<String, String> replace) {
        if ((_artistOrderName == null) || (_artistOrderName.length() == 0)) {
            
            _artistOrderName = _artistName;
            
            int firstSpace = _artistName.indexOf(" ");
            
            if (firstSpace != -1) {
                String firstWord = _artistName.substring(0, firstSpace).toLowerCase();
                String[] words = Options.getIgnoreAlpha();
                for (int i=0; i<words.length; i++) {
                    if (words[i].compareTo(firstWord) == 0) {
                        _artistOrderName = _artistName.substring(firstSpace + 1);
                    }
                }
            }
            
            for (@SuppressWarnings("rawtypes")Iterator it=replace.entrySet().iterator(); it.hasNext(); ) {
            	@SuppressWarnings("unchecked")
				Map.Entry<String, String> entry = (Map.Entry<String, String>)it.next();
            	_artistOrderName = _artistOrderName.replace(entry.getKey(), entry.getValue());
            }            
        }
    }
 
    public void toJson(StringBuffer sb, int childLevel) {
    	sb.append("{ ");
    	appendJsonElement(sb, "artist_id", _artistID);
    	sb.append(", ");
    	appendJsonElement(sb, "name", _artistName);
    	sb.append(", ");
    	appendJsonElement(sb, "order_name", _artistOrderName);
    	sb.append(", ");
    	appendJsonElement(sb, "album_count", _albumCount);
    	sb.append(", ");
    	appendJsonElement(sb, "track_count", _trackCount);
    	if (childLevel > 0) {
    		AlbumCollection list = new AlbumCollection();
    		list.listByArtist(_artistID, 0, 0);
    		sb.append(", ");
    		appendJsonElement(sb, "albums", list, --childLevel);
    	}
    	sb.append("}");
    }
    
}