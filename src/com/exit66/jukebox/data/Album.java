package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.io.DataOutputStream;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

import com.exit66.jukebox.Options;

public class Album extends BVDatabaseObject {
    
    private int		_albumID = -1;
    private String	_albumName;
    private int		_multipleArtistID = -1;
    private Artist  _multipleArtist = null;
    private int     _artistCount = 0;
    private int		_trackCount = 0;
    
    public int getAlbumID() {
        
        return _albumID;
        
    }
    
    public void setAlbumID(int newValue) {
        
        _albumID = newValue;
        
    }
    
    public String getAlbumName() {
        
        return _albumName;
        
    }
    
    public void setAlbumName(String newValue) {
        
        if (newValue != null)
            _albumName = newValue.trim();
        else
            _albumName = null;
        
    }
    
    public Artist getMultipleArtist() {
        if ((_multipleArtist == null) || (_multipleArtistID != _multipleArtist.getArtistID())) {
            if (_multipleArtistID != -1) {
                _multipleArtist = new Artist();
                _multipleArtist.setArtistID(_multipleArtistID);
                _multipleArtist.fetch();
            } else {
                _multipleArtist = new Artist();
            }
        }
        
        return _multipleArtist;
    }
    
    public int getMultipleArtistID() {
        
        return _multipleArtistID;
        
    }
    
    public void setMultipleArtistID(int newValue) {
        
        _multipleArtistID = newValue;
        
    }
    
    public void setArtistCount(int newValue) {
        _artistCount = newValue;
    }
    
    public int getArtistCount() {
        return _artistCount;
    }
    
    public void setTrackCount(int newValue) {
        _trackCount = newValue;
    }
    
    public int getTrackCount() {
        return _trackCount;
    }
    
    public boolean fetch() {
        
        if (openConnection()) {
            
            try {
            	
            	PreparedStatement ps = conn.prepareStatement("SELECT * FROM album WHERE album_id = ?");
            	ps.setInt(1, _albumID);
                ResultSet rs = retrieveData(ps);
                
                if (rs.next()) {
                    
                    _albumName = rs.getString("album_name");
                    _multipleArtistID = rs.getInt("multiple_artist_id");
                    _artistCount = rs.getInt("artist_count");
                    _trackCount = rs.getInt("track_count");
                    
                } else {
                    
                    _albumName = "";
                    _multipleArtistID = -1;
                    _artistCount = 0;
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
    
    public void save(boolean findAlbum) {
        
        ResultSet rs;
        
        if (openConnection()) {
            
            try {

                if ((findAlbum == true) && (_albumID == -1)) {
                    
                    this.find();
                    
                }
                
                if (_albumID == -1) {
                    
                    try {
                    
                    	conn.setAutoCommit(false);
                    	PreparedStatement ps = conn.prepareStatement("INSERT INTO album (album_name, multiple_artist_id, " + 
                    			"track_count) VALUES (?, ?, ?)");
                    	ps.setString(1, _albumName);
                    	ps.setInt(2, _multipleArtistID);
                    	ps.setInt(3, _trackCount);
                    	executeStatement(ps);
                    	
                    	rs = retrieveData("CALL IDENTITY()");
                        
                        if (rs.next()) {
                            
                            _albumID = rs.getInt(1);
                            
                        }
                        
                        conn.commit();
                        conn.setAutoCommit(true);
                        
                    } catch (SQLException e) {
                        
                        System.err.println(e);
                        
                    }
                    
                } else {
                    
                	PreparedStatement ps = conn.prepareStatement("UPDATE album SET album_name = ?, " +
                            "multiple_artist_id = ?, track_count = ? WHERE album_id = ?");
                	ps.setString(1, _albumName);
                	ps.setInt(2, _multipleArtistID);
                	ps.setInt(3, _trackCount);
                	ps.setInt(4, _albumID);
                	executeStatement(ps);
                    
                }
            } catch (Exception e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
        }
        
    }
    
    public void save() {
        
        save(true);
    }
    
    public boolean find() {
        
        openConnection();
        
        try {
        
        	PreparedStatement ps = conn.prepareStatement("SELECT * FROM album WHERE album_name = ?");
        	ps.setString(1, _albumName);
        	ResultSet rs = retrieveData(ps);
                        
            if (rs != null && rs.next()) {
                
                closeConnection();
                _albumID = rs.getInt("album_id");
                _multipleArtistID = rs.getInt("multiple_artist_id");
                _artistCount = rs.getInt("artist_count");
                _trackCount = rs.getInt("track_count");
                return true;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return false;
        
    }
    
    public boolean findForArtist(int artistID) {
        
        openConnection();
        
        try {
        	
        	PreparedStatement ps = conn.prepareStatement("SELECT album.* FROM album JOIN artistalbumlink " + 
        		"ON album.album_id = artistalbumlink.album_id WHERE album_name = ? " + 
        		" AND artistalbumlink.artist_id = ?");
        	ps.setString(1, _albumName);
        	ps.setInt(2, artistID);
        	ResultSet rs = retrieveData(ps);
            
            if (rs != null && rs.next()) {
                
                closeConnection();
                _albumID = rs.getInt("album_id");
                _multipleArtistID = rs.getInt("multiple_artist_id");
                _artistCount = rs.getInt("artist_count");
                _trackCount = rs.getInt("track_count");
                return true;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return false;
        
    } 
    
    public int findArtistID() {
        
        int count = 0;
        int artistID = -1;
        
        if (_multipleArtistID != -1) {
            
            return _multipleArtistID;
            
        }
        
        try {
        
        	PreparedStatement ps = conn.prepareStatement("SELECT DISTINCT artist_id FROM track WHERE album_id = ?");
        	ps.setInt(1, _albumID);
        	ResultSet rs = retrieveData(ps);
                        
            while (rs.next()) {
                
                artistID = rs.getInt("artist_id");
                count++;
                
            }
            
            if (count != 1) {
                
                return -1;
                
            } else {
                
                return artistID;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            return -1;
            
        }
        
    }
    
    public void split() {
        
        fetch();
        
        try {
        	PreparedStatement ps = conn.prepareStatement("SELECT DISTINCT artist_id FROM artistalbumlink WHERE album_id = ?");
        	ps.setInt(1, _albumID);
        	ResultSet rs = retrieveData(ps);
            
            Album album = new Album();
            
            rs.next(); // skip the first one, we're leaving it as is
            
            while (rs.next()) {
                
                album.setAlbumID(-1);
                album.setAlbumName(_albumName);
                album.save(false);

                ps = conn.prepareStatement("UPDATE artistalbumlink SET album_id = ? " +
                        " WHERE album_id = ? AND artist_id = ?");
                ps.setInt(1, album.getAlbumID());
                ps.setInt(2, _albumID);
                ps.setInt(3, rs.getInt("artist_id"));
                executeStatement(ps);
                
                ps = conn.prepareStatement("UPDATE track SET album_id = ? " +
		                " WHERE album_id = ? AND artist_id = ?");
		        ps.setInt(1, album.getAlbumID());
		        ps.setInt(2, _albumID);
		        ps.setInt(3, rs.getInt("artist_id"));
		        executeStatement(ps);
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
    }
    
    public Track getFirstTrackForAlbum() {
        
        try {
            
        	PreparedStatement ps = conn.prepareStatement("SELECT track_id FROM track WHERE album_id = ? " +
                    "ORDER BY track_number, track_name");
        	ps.setInt(1, _albumID);
        	ResultSet rs = retrieveData(ps);
            
            if (rs.next()) {
                
                Track t = new Track();
                
                t.setTrackID(rs.getInt("track_id"));
                t.fetch();
                return t;
                
            }
            
        } catch (Exception e) {
            
            System.err.println(e);
            
        }
        
        return null;
        
    }
    
    public void getCoverImage(DataOutputStream os, boolean showNotFound) {
        
        try {
            
        	PreparedStatement ps = conn.prepareStatement("SELECT track_id FROM track WHERE album_id = ? " +
		            "ORDER BY track_number, track_name");
			ps.setInt(1, _albumID);
			ResultSet rs = retrieveData(ps);
            
            if (rs.next()) {
                
                Track t = new Track();
                
                t.setTrackID(rs.getInt("track_id"));
                t.fetch();
                t.getCoverImage(os, showNotFound);
                return;
                
            }
            
        } catch (Exception e) {
            
            System.err.println(e);
            
        }
        
        if (showNotFound) {
            Options.outputDefaultImage(os);
        }
        
    }
    
    public void toJson(StringBuffer sb, int childLevel) {    	
    	sb.append("{ ");
    	appendJsonElement(sb, "album_id", _albumID);
    	sb.append(", ");
    	appendJsonElement(sb, "name", _albumName);
    	sb.append(", ");
    	appendJsonElement(sb, "multiple_artist_id", _multipleArtistID);
    	sb.append(", ");
    	appendJsonElement(sb, "track_count", _trackCount);
    	sb.append(", ");
    	if (_multipleArtistID != -1) {
        	Artist multipleArtist = getMultipleArtist();
    		appendJsonElement(sb, "multiple_artist_name", multipleArtist.getArtistName());
    	} else {
    		appendJsonElement(sb, "multiple_artist_name", "");
    	}
    	if (childLevel > 0) {
    		TrackCollection list = new TrackCollection();
    		list.listByAlbum(_albumID, 0, 0);
    		sb.append(", ");
    		appendJsonElement(sb, "tracks", list, --childLevel);
    	}
    	sb.append("}");
    }  
    
}
