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

import com.exit66.jukebox.Options;

public class RequestQueue extends BVDatabaseObject {
    
    public static final int TRACK_REQUESTED = 0;
    public static final int TRACK_ALREADY_QUEUED = 1;
    public static final int TRACK_RECENTLY_PLAYED = 2;
    public static final int ALBUM_ALL_TRACKS_REQUESTED = 3;
    public static final int ALBUM_SOME_TRACKS_REQUESTED = 4;
    public static final int ALBUM_NO_TRACKS_REQUESTED = 5;
    public static final int ARTIST_ALL_TRACKS_REQUESTED = 6;
    public static final int ARTIST_SOME_TRACKS_REQUESTED = 7;
    public static final int ARTIST_NO_TRACKS_REQUESTED = 8;
    
    public static String getQueueMessage(int messageID) {
    	switch (messageID) {
        case TRACK_ALREADY_QUEUED: return "That track has already been requested.\\nTry requesting a different song.";
        case TRACK_RECENTLY_PLAYED: return "That track has recently been played.\\nTry requesting a different song.";
        case TRACK_REQUESTED: return "Thanks for the request!";
        case ALBUM_ALL_TRACKS_REQUESTED:
        case ARTIST_ALL_TRACKS_REQUESTED:
            return "All the tracks have been requested.";
        case RequestQueue.ALBUM_SOME_TRACKS_REQUESTED:
        case RequestQueue.ARTIST_SOME_TRACKS_REQUESTED:
            return "Some of the tracks have been requested.\\nOthers have already been requested or have been recently played.";
        case RequestQueue.ALBUM_NO_TRACKS_REQUESTED:
        case RequestQueue.ARTIST_NO_TRACKS_REQUESTED:
            return "None of the tracks have been requested.\\nThey are already been requested or have been recently played.";
    	}
    	return "Invalid message id";
    }
           
    public int requestTrack(String sessionID, int trackID) {
        
        int nextOrder = 1;
        
        openConnection();
        
        // we're not overly concerned with duplicate play orders so don't
        // worry about multiple transactions
        
        
        try {

        	PreparedStatement ps = conn.prepareStatement("SELECT COUNT(track_id) FROM request_queue WHERE track_id = ? " +
        			"AND session_id = ?");
        	ps.setInt(1, trackID);
        	ps.setString(2, sessionID);
        	ResultSet rs = retrieveData(ps);
                
            rs.next();
            
            if (rs.getInt(1) == 0) {
            
            	ps = conn.prepareStatement("SELECT COUNT(track_id) FROM history " + 
            			"WHERE track_id = ? AND session_id = ? AND track_id IN (SELECT TOP " + 
            			Options.getHistoryCount() + " track_id FROM history ORDER BY entry)");
            	ps.setInt(1, trackID);
            	ps.setString(2, sessionID);
            	rs = retrieveData(ps);
                
                rs.next();
                
                if (rs.getInt(1) == 0) {
                    
                	ps = conn.prepareStatement("SELECT MAX(play_order) FROM request_queue WHERE session_id = ?");
                	ps.setString(1, sessionID);
                    rs = retrieveData(ps);
                    
                    rs.next();
                    
                    if (rs.getString(1) != null) {
                        
                        nextOrder = rs.getInt(1) + 1;
                        
                    }
                    
                    ps = conn.prepareStatement("INSERT INTO request_queue (track_id, session_id, play_order) VALUES (?, ?, ?)");
                    ps.setInt(1, trackID);
                    ps.setString(2, sessionID);
                    ps.setInt(3, nextOrder);
                    executeStatement(ps);
                }
                else 
                {
                    return TRACK_RECENTLY_PLAYED;
                }
            }
            else 
            {
                return TRACK_ALREADY_QUEUED;
            }
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        closeConnection();
        return TRACK_REQUESTED;
        
    }
    
    public int requestAlbum(String sessionID, int albumID) {
        
        int result = ALBUM_NO_TRACKS_REQUESTED;
        boolean failedRequest = false;
        openConnection();
        
        try {
        	PreparedStatement ps = conn.prepareStatement("SELECT track_id FROM track WHERE album_id = ? " +
        			"ORDER BY track_number, track_name");
        	ps.setInt(1, albumID);
        	ResultSet rs = retrieveData(ps);
                            
            while (rs.next()) {
                
                if (requestTrack(sessionID, rs.getInt("track_id")) == TRACK_REQUESTED) {
                    if (!failedRequest) {
                        result = ALBUM_SOME_TRACKS_REQUESTED;
                    }
                }
                else {
                    failedRequest = true;
                }                
            }
            
            if (!failedRequest) {
                result = ALBUM_ALL_TRACKS_REQUESTED;
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        return result;
    }
    
    public int requestArtist(String sessionID, int artistID) {
        
        int result = ARTIST_NO_TRACKS_REQUESTED;
        boolean failedRequest = false;
        openConnection();
        
        try {
        	PreparedStatement ps = conn.prepareStatement("SELECT track_id FROM track WHERE artist_id = ?");
			ps.setInt(1, artistID);
			ResultSet rs = retrieveData(ps);
	
            while (rs.next()) {
                
                if (requestTrack(sessionID, rs.getInt("track_id")) == TRACK_REQUESTED) {
                     if (!failedRequest) {
                        result = ARTIST_SOME_TRACKS_REQUESTED;
                    }
                }
                else {
                    failedRequest = true;
                }
                
            }
            
            if (!failedRequest) {
                result = ARTIST_ALL_TRACKS_REQUESTED;
            }
            
        } catch (SQLException e) {
            
            System.err.println(e);
            
        }
        
        return result;
        
    }
    
    public void removeRequest(String sessionID, int trackID) {
        
        if (openConnection() == true) {
            
	        try {
	        	
	        	PreparedStatement ps = conn.prepareStatement("DELETE FROM request_queue WHERE track_id = ? AND session_id = ?");
	            ps.setInt(1, trackID);
	            ps.setString(2, sessionID);
	            executeStatement(ps);
	            
        	} catch (SQLException e) {
        		
        		System.err.println(e);
        		
        	}
            closeConnection();
            
        }
        
    }
    
    public void clear(String sessionID) {
        
        if (openConnection() == true) {
            
	        try {
	        	
	        	PreparedStatement ps = conn.prepareStatement("DELETE FROM request_queue WHERE session_id = ?");
	        	ps.setString(1, sessionID);
	            executeStatement(ps);
	            
        	} catch (SQLException e) {
        		
        		System.err.println(e);
        		
        	}
            closeConnection();
            
        }
        
    }
    
    public void removeRequests(String sessionID, String[] tracks) {
        
        for (int i=0; i<tracks.length; i++) {
            
            if (tracks[i].length() != 0)
                removeRequest(sessionID, Integer.parseInt(tracks[i]));
            
        }
        
        
    }
        
    public int getNextRequest(String sessionID) {
        
        int trackID = -1;
        
        if (openConnection() == true) {
            
            try {

            	PreparedStatement ps;
            	if (Options.getRandomCount() < 2) {
	            	ps = conn.prepareStatement("select top 1 track_id from request_queue " + 
	            			"where session_id = ? order by play_order");
            	} else {
            		ps = conn.prepareStatement("select top 1 track_id from (select top " + 
	            			Options.getRandomCount() + " * from request_queue order by play_order) " + 
	            			"where session_id = ? order by rand()");
            	}
            	ps.setString(1, sessionID);
                ResultSet rs = retrieveData(ps);
                
                if (rs.next()) {
                    
                    trackID = rs.getInt("track_id");
                    
                    ps = conn.prepareStatement("DELETE FROM request_queue WHERE track_id = ? AND session_id = ?");
    	            ps.setInt(1, trackID);
    	            ps.setString(2, sessionID);
    	            executeStatement(ps);
    	            
    	            ps = conn.prepareStatement("INSERT INTO history (track_id, session_id, entry) VALUES (?, ?, NOW())");
    	            ps.setInt(1, trackID);
    	            ps.setString(2, sessionID);
    	            executeStatement(ps);
    	            
    	            ps = conn.prepareStatement("UPDATE track SET playcount = playcount + 1 WHERE track_id = ?");
    	            ps.setInt(1, trackID);
    	            executeStatement(ps);
                }
                
            } catch (SQLException e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
        }
        
        return trackID;
        
    }

    public void toJson(StringBuffer sb, int childLevel) {
    	
    }
}
