package com.exit66.jukebox.data;

import java.sql.PreparedStatement;
import java.sql.SQLException;

public class ArtistCollection extends BVDatabaseCollection {
    
    public ArtistCollection() {
        
        _currentRecord = new Artist();
        
    }
    
    public void list(int start, int count) {
        
    	list(false, start, count);
        
    }
    
    public void list(boolean multipleOnly, int start, int count) {
    
        String sql = "";
        if (openConnection()) {
            
        	if (count == 0) 
        		sql = "SELECT ";
        	else
        		sql = "SELECT LIMIT " + start + " " + count;
        	sql = sql + " * FROM artist ";
        	if (multipleOnly) {
        		sql = sql + " WHERE artist_id IN " +
        			"(SELECT multiple_artist_id FROM album)"; 
        	}
        	
            _rs = retrieveData(sql + " ORDER BY artist_order_name");
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
            	
            	PreparedStatement ps = conn.prepareStatement(sql + " * FROM artist WHERE artist_name LIKE ? OR " +
            			"artist_order_name LIKE ? ORDER BY artist_order_name");
            	ps.setString(1, search);
            	ps.setString(2, search);            	
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
            
            ((Artist)_currentRecord).setArtistID(_rs.getInt("artist_id"));
            ((Artist)_currentRecord).setArtistName(_rs.getString("artist_name"));
            ((Artist)_currentRecord).setArtistOrderName(_rs.getString("artist_order_name"));
            ((Artist)_currentRecord).setAlbumCount(_rs.getInt("album_count"));
            ((Artist)_currentRecord).setTrackCount(_rs.getInt("track_count"));
            
        } catch (SQLException e) {};
        
    }
    
}
