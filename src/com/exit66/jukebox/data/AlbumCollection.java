package com.exit66.jukebox.data;

import java.sql.PreparedStatement;
import java.sql.SQLException;

public class AlbumCollection extends BVDatabaseCollection {
        
    public AlbumCollection() {
        
        _currentRecord = new Album();
        
    }
    
 public void list(int start, int count) {
        
        String sql = "";
        if (openConnection()) {
            
        	if (count == 0) 
        		sql = "SELECT ";
        	else
        		sql = "SELECT LIMIT " + start + " " + count;
        	
            _rs = retrieveData(sql + " * FROM album ORDER BY album_name");
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
            	
            	PreparedStatement ps = conn.prepareStatement(sql + " * FROM album WHERE album_name LIKE ? " +
            			"ORDER BY album_name");
            	ps.setString(1, search);     	
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
            	
            	PreparedStatement ps = conn.prepareStatement(sql + " album.* FROM album " + 
            			"WHERE album_id IN (SELECT album_id FROM artistalbumlink WHERE artist_id = ?) " +
            			" OR album_id IN (SELECT album_id FROM album WHERE multiple_artist_id = ?) " +
            			"ORDER BY album_name");
            	ps.setInt(1, artistID);
            	ps.setInt(2, artistID);
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
    
    public void listCompilations() {
        
        String sql = "SELECT album.* FROM album WHERE (multiple_artist_id != -1 AND multiple_artist_id != null) ORDER BY album_name ";
        
        if (openConnection()) {
            
            _rs = retrieveData(sql);
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
    
    public void listPossibleCompilations() {
        
        updateAlbumArtistCount();
        
        String sql = "SELECT album.* FROM album WHERE artist_count > 1 " +
                "AND (multiple_artist_id = -1 OR multiple_artist_id = null)  ORDER BY album_name";
        
        if (openConnection()) {
            
            _rs = retrieveData(sql);
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
    
    protected void loadCurrent() {
        
        try {
            
            ((Album)_currentRecord).setAlbumID(_rs.getInt("album_id"));
            ((Album)_currentRecord).setAlbumName(_rs.getString("album_name"));
            try {
                ((Album)_currentRecord).setArtistCount(_rs.getInt("artist_count"));
            }
            catch (Exception e) {
                ((Album)_currentRecord).setArtistCount(0);
            }
            ((Album)_currentRecord).setMultipleArtistID(_rs.getInt("multiple_artist_id"));
            ((Album)_currentRecord).setTrackCount(_rs.getInt("track_count"));
            
        } catch (SQLException e) {};
        
    }
    
    public void updateAlbumArtistCount() {
        
        String sql = "UPDATE album SET artist_count = (SELECT artist_count FROM vw_albumartistcount WHERE album.album_id = vw_albumartistcount.album_id)";
        
        executeStatement(sql);
                
    }
    
}
