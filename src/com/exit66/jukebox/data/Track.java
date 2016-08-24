package com.exit66.jukebox.data;

import java.io.DataOutputStream;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

import com.exit66.jukebox.Options;
import com.exit66.jukebox.util.MediaFile;

public class Track extends BVDatabaseObject {
    
    private int	_trackID = -1;
    private int	_libraryID = -1;
    private String	_fileName;
    private String 	_trackName;
    private int	_albumID = -1;
    private int	_artistID = -1;
    private int	_contentTypeID = -1;
    private int	_trackNumber;
    private String _time;
    private int _playcount = 0; 
    
    private Artist  _artist = null;
    private Album   _album = null;
    private Library _library = null;
    
    public int getTrackID() {
        
        return _trackID;
        
    }
    
    public void setTrackID(int newValue) {
        
        _trackID = newValue;
        
    }
    
    public int getLibraryID() {
        
        return _libraryID;
        
    }
    
    public void setLibraryID(int newValue) {
        
        _libraryID = newValue;
        
    }
    
    public String getFileName() {
        
        return _fileName;
        
    }
    
    public String getFullFileName() {
    	Library lib = new Library();
    	lib.setLibraryID(_libraryID);
    	if (lib.fetch()) {
    		return lib.getLibraryPath() + _fileName;
    	}
    	else {
    		return "";
    	}
    }
    
    public void setFileName(String newValue) {
        
        if (newValue != null)
            _fileName = newValue;
        else
            _fileName = null;
        
    }
    
    public String getTrackName() {
        
        return _trackName;
        
    }
    
    public void setTrackName(String newValue) {
        
        if (newValue != null)
            _trackName = newValue;
        else
            _trackName = null;
        
    }
    
    public int getAlbumID() {
        
        return _albumID;
        
    }
    
    public void setAlbumID(int newValue) {
        
        _albumID = newValue;
        
    }
    
    public int getArtistID() {
        
        return _artistID;
        
    }
    
    public void setArtistID(int newValue) {
        
        _artistID = newValue;
        
    }
    
    public void setArtist(Artist artist) {
    	_artist = artist;
    }
    
    public void setAlbum(Album album) {
    	_album = album;
    }
    
    public int getContentTypeID() {
        
        return _contentTypeID;
        
    }
    
    public void setContentTypeID(int newValue) {
        
        _contentTypeID = newValue;
        
    }
    
    public int getTrackNumber() {
        
        return _trackNumber;
        
    }
    
    public void setTrackNumber(int newValue) {
        
        _trackNumber = newValue;
        
    }
    
    public String getTime() {
        
        return _time;
        
    }
    
    public void setTime(String newValue) {
        
        if (newValue != null)
            _time = newValue;
        else
            _time = null;
        
    }

    public int getPlaycount() {
        
        return _playcount;
        
    }
    
    public void setPlaycount(int newValue) {
        
        _playcount = newValue;
        
    }
    public Artist getArtist() {
        if ((_artist == null) || (_artistID != _artist.getArtistID())) {
            if (_artistID != -1) {
                _artist = new Artist();
                _artist.setArtistID(_artistID);
                _artist.fetch();
            } else {
                _artist = new Artist();
            }
        }
        
        return _artist;
    }
    
    public Album getAlbum() {
        if ((_album == null) || (_albumID != _album.getAlbumID())) {
            if (_albumID != -1) {
                _album = new Album();
                _album.setAlbumID(_albumID);
                _album.fetch();
            } else {
                _album = new Album();
            }
        }
        
        return _album;
    }
    
    public Library getLibrary() {
        if ((_library == null) || (_libraryID != _library.getLibraryID())) {
            if (_libraryID != -1) {
                _library = new Library();
                _library.setLibraryID(_libraryID);
                _library.fetch();
            } else {
                _library = new Library();
            }
        }
        
        return _library;
    }
    
    public boolean fetch() {
        
        if (openConnection()) {
            
            try {

            	PreparedStatement ps = conn.prepareStatement("SELECT * FROM track WHERE track_id = ?");
            	ps.setInt(1, _trackID);
            	ResultSet rs = retrieveData(ps);

                if (rs != null && rs.next()) {
                    
                    _libraryID = rs.getInt("library_id");
                    _fileName = rs.getString("file_name");
                    _trackName = rs.getString("track_name");
                    _artistID = rs.getInt("artist_id");
                    _albumID = rs.getInt("album_id");
                    _contentTypeID = rs.getInt("content_type_id");
                    _trackNumber = rs.getInt("track_number");
                    _time = rs.getString("track_time");
                    _playcount = rs.getInt("playcount");
                    
                } else {
                    
                    _libraryID = -1;
                    _fileName = "";
                    _trackName = "";
                    _artistID = -1;
                    _albumID = -1;
                    _contentTypeID = -1;
                    _trackNumber = -1;
                    _time = "";
                    _playcount = 0;
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
        
        if (openConnection()) {
            
            if (_trackID == -1) {
                
                this.find();
                
            }
            
            if (_trackID == -1) {
                
            	
                try {
                
                	conn.setAutoCommit(false);
                	PreparedStatement ps = conn.prepareStatement("INSERT INTO track (library_id, file_name, " +
                			"track_name, artist_id, album_id, content_type_id, track_number, playcount, track_time) VALUES " +
                			"(?, ?, ?, ?, ?, ?, ?, ?, ?)");
                	ps.setInt(1, _libraryID);
                	ps.setString(2, _fileName);
                	ps.setString(3, _trackName);
                	ps.setInt(4, _artistID);
                	ps.setInt(5, _albumID);
                	ps.setInt(6, _contentTypeID);
                	ps.setInt(7, _trackNumber);
                	ps.setInt(8, _playcount);
                	ps.setString(9, _time);
                	executeStatement(ps);
                	
                	rs = retrieveData("CALL IDENTITY()");
                    if (rs.next()) {
                        
                        _trackID = rs.getInt(1);
                        
                    }
                    
                    conn.commit();
                    conn.setAutoCommit(true);
                    
                } catch (SQLException e) {
                    
                    System.err.println(e);
                    
                }
                
            } else {
                
            	try {
	            	PreparedStatement ps = conn.prepareStatement("UPDATE track SET library_id = ?, " +
	                        "file_name = ?, track_name = ?, artist_id = ?, album_id = ?, " +
	                        "content_type_id = ?, track_number = ?, track_time = ?, playcount = ? WHERE track_id = ?");
	            	ps.setInt(1, _libraryID);
	            	ps.setString(2, _fileName);
	            	ps.setString(3, _trackName);
	            	ps.setInt(4, _artistID);
	            	ps.setInt(5, _albumID);
	            	ps.setInt(6, _contentTypeID);
	            	ps.setInt(7, _trackNumber);
	            	ps.setString(8, _time);
	            	ps.setInt(9, _playcount);
	            	ps.setInt(10, _trackID);
	            	executeStatement(ps);
	            	
            	} catch (SQLException e) {
            		
            		System.err.println(e);
            		
            	}
                
            }
            
        	try {
                PreparedStatement ps = conn.prepareStatement("DELETE FROM library_failed_files WHERE library_id = ?" + 
                        " AND file_name = ?");
            	ps.setInt(1, _libraryID);
            	ps.setString(2, _fileName);
            	executeStatement(ps);
            	
        	} catch (SQLException e) {
        		
        		System.err.println(e);
        		
        	}
            
            closeConnection();
            
        }
        
    }
    
    public boolean find() {
        
        openConnection();
        
        try {
        	
        	PreparedStatement ps = conn.prepareStatement("SELECT track_id, track_name, artist_id, album_id, content_type_id, track_number, track_time, playcount " +
                    " FROM track WHERE file_name = ? AND library_id = ?");
        	ps.setString(1, _fileName);
        	ps.setInt(2, _libraryID);
        	ResultSet rs = retrieveData(ps);
            
            if (rs.next()) {
                
                closeConnection();
                _trackID = rs.getInt("track_id");
                _trackName = rs.getString("track_name");
                _artistID = rs.getInt("artist_id");
                _albumID = rs.getInt("album_id");
                _contentTypeID = rs.getInt("content_type_id");
                _trackNumber = rs.getInt("track_number");
                _time = rs.getString("track_time");
                _playcount = rs.getInt("playcount");
                return true;
                
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return false;
        
    }
    
    public void playTrack() {
    	
    	openConnection();
                
    	try {
    		
    		PreparedStatement ps = conn.prepareStatement("INSERT INTO history (track_id, entry) VALUES (?, NOW())");
    		ps.setInt(1, _trackID);
	        executeStatement(ps);
	        
	        int maxHistory = Math.max(Options.getHistoryCount(), Options.getPlaylistHistoryCount());
	        
	        executeStatement("DELETE FROM history WHERE track_id NOT IN (SELECT TOP " + maxHistory + " track_id FROM history ORDER BY entry DESC)");
	        
	        ps = conn.prepareStatement("UPDATE track SET playcount = playcount + 1 WHERE track_id = ?");
	        ps.setInt(1, _trackID);
	        executeStatement(ps);
	    	
    	} catch (SQLException e) {
    		
    		System.err.println(e);
    		
    	}
    	
        closeConnection();
    }
    
    public void getCoverImage(DataOutputStream os, boolean showNotFound) {
        
        MediaFile mf = new MediaFile(_fileName);
        mf.setLibraryID(_libraryID);
        mf.setFileName(_fileName);
        mf.getCoverImage(os, showNotFound);
        
    }
    
    public void toJson(StringBuffer sb, int childLevel) {
    	getAlbum();
    	getArtist();
    	
    	sb.append("{ ");
    	appendJsonElement(sb, "track_id", _trackID);
    	sb.append(", ");
    	appendJsonElement(sb, "name", _trackName);
    	sb.append(", ");
    	appendJsonElement(sb, "number", _trackNumber);
    	sb.append(", ");
    	appendJsonElement(sb, "playcount", "0");  // TODO include playcount
    	sb.append(", ");
    	appendJsonElement(sb, "album_id", _albumID);
    	sb.append(", ");
    	appendJsonElement(sb, "album_name", _album.getAlbumName());
    	sb.append(", ");
    	appendJsonElement(sb, "artist_id", _artistID);
    	sb.append(", ");
    	appendJsonElement(sb, "artist_name", _artist.getArtistName());
    	sb.append(", ");
    	appendJsonElement(sb, "time", _time);
    	sb.append(", ");
    	appendJsonElement(sb, "playcount", _playcount);
    	sb.append("}");
    }
}
