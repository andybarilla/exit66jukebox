package com.exit66.jukebox.util;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.io.DataOutputStream;
import java.io.File;
import java.io.FileInputStream;

import com.exit66.jukebox.Options;
import com.exit66.jukebox.data.Album;
import com.exit66.jukebox.data.Artist;
import com.exit66.jukebox.data.ArtistAlbumLink;
import com.exit66.jukebox.data.ContentType;
import com.exit66.jukebox.data.Library;
import com.exit66.jukebox.data.Track;
import com.exit66.jukebox.tag.*;

public class MediaFile {
    
    private int			_libraryID;
    private String		_libraryPath;
    private String 		_fileName;
    private static Lock _findAndSaveLock = new Lock();
    
    public void setLibraryID(int newValue) {
        
        _libraryID = newValue;
        Library l = new Library();
        l.setLibraryID(_libraryID);
        l.fetch();
        _libraryPath = l.getLibraryPath();
        
    }
    
    public int getLibraryID() {
        
        return _libraryID;
        
    }
    
    public void setLibraryPath(String newValue) {
        
        _libraryPath = newValue;
        
    }
    
    public String getLibraryPath() {
        
        return _libraryPath;
        
    }
    
    public MediaFile(String newFileName) {
        
        _fileName = newFileName;
        
    }
    
    public MediaFile(File file) {
        
        _fileName = file.getAbsolutePath();
        
    }
    
    public void setFileName(String newValue) {
        
        _fileName = newValue;
        
    }
    
    public String getFileName() {
        
        return _fileName;
        
    }
    
    public boolean importTagInfo()
    {
        TagReader tag = null;
        Artist artist = new Artist();
        Artist albumArtist = new Artist();
        Album album = new Album();
        ArtistAlbumLink artistalbumlink = new ArtistAlbumLink();
        Track track = new Track();
        ContentType contenttype = new ContentType();
    	tag = TagReaderFactory.getTagReader(_fileName);
               
        if (tag != null)
        {
        	tag.readTag(_fileName);
        	 
            track.setLibraryID(_libraryID);
            track.setFileName(_fileName.substring(_libraryPath.length()));
            track.find();
            
            try {
            	
            	_findAndSaveLock.lock();
            
	            if (tag.getArtistName() != null && tag.getArtistName().length() != 0)
	            	artist.setArtistName(tag.getArtistName());
	            else 
	            	artist.setArtistName(Options.getUnknownArtist());
	           
	            if(!artist.find())
	            {
	                artist.guessArtistOrderName();
	                artist.save();
	            }
	            
	            // look for the album artist, if there isn't one use the track artist 
	            if (tag.getAlbumArtistName() != null && tag.getAlbumArtistName().length() != 0) {
	            	albumArtist.setArtistName(tag.getAlbumArtistName());
	            	
	            	if(!albumArtist.find())
	                {
	                	albumArtist.guessArtistOrderName();
	                	albumArtist.save();
	                }
	            } else { 
	            	albumArtist = artist;
	            }
	            
	            // allow duplicate albums of the same name if the album is unknown
	            if (tag.getAlbumName() != null && tag.getAlbumName().length() != 0) {
	            	album.setAlbumName(tag.getAlbumName());
	            	album.setMultipleArtistID(albumArtist.getArtistID());
	            	if(!album.find())
	                    album.save();
	            }
	            else {
	            	album.setAlbumName(Options.getUnknownAlbum());
	            	if (!album.findForArtist(artist.getArtistID()))
	            		album.save(false);
	            }
	            
	            artistalbumlink.setArtistID(artist.getArtistID());
	            artistalbumlink.setAlbumID(album.getAlbumID());
	            if(!artistalbumlink.find())
	                artistalbumlink.save();
	            
	            if (tag.getContentType() != null && tag.getContentType().length() != 0)
	            	contenttype.setContentType(tag.getContentType());
	            else
	            	contenttype.setContentType(Options.getUnknownContent());
	            if(!contenttype.find())
	                contenttype.save();
	            
            } finally {
            	_findAndSaveLock.releaseLock();
            }
            
            
            if (tag.getTrackName() != null && tag.getTrackName().length() != 0) {
            	track.setTrackName(tag.getTrackName());
            }
            else {
            	track.setTrackName(getFileName(_fileName));
            }
            track.setTrackNumber(Math.max(tag.getTrackNumber(), 0));
            track.setAlbumID(album.getAlbumID());
            track.setArtistID(artist.getArtistID());
            track.setContentTypeID(contenttype.getContentTypeID());
            track.save();
            return true;
        } 
        else
        {
            return false;
        }
    }
    
    private String getFileName(String fileName) {
        
    	String out;
        int loc = fileName.lastIndexOf(".");
        
        if	(loc >= 0) {
            
            out = fileName.substring(0, loc);
            
        } else {
            
            out = fileName;
            
        }
        
        loc = out.lastIndexOf(File.separatorChar);
        
        if (loc >= 0) {
        	out = out.substring(loc+1);
        }
        
        return out;
        
    }
    
    public void getCoverImage(DataOutputStream os, boolean showNotFound) {
        
        TagReader tag = null;
        boolean tagHadImage = false;
        String extension = Options.getExtension(_fileName).toString().toUpperCase();
        
        if (extension.compareTo("MP3") == 0) {
            
        	tag = TagReaderFactory.getTagReader(_fileName);
            tag.setFileName(_libraryPath.concat(_fileName));
            if (tag.loadCoverImage()) {
            	tagHadImage = true;
            	try {
	                os.writeBytes("HTTP/1.1 200 \r\n");
	                os.writeBytes("Content-type: " + tag.getCoverMimeType() + "\r\n");
	                os.writeBytes("Connection: close\r\n");
	
	                os.writeBytes("\r\n");
	                os.write(tag.getCoverImage());
            	}
            	catch (Exception e) {
            		System.err.println(e);
            		tagHadImage = false;
            	}
            }
            
        }
        
        if ((tagHadImage == false) && (showNotFound)) {
            
            try {
                if (Options.getUseAlbumImageFile().compareTo("1") == 0) {
                    
                    // search for an image file in the directory
                    // of the song file
                    File file = new File(_libraryPath, _fileName);
                    ImageFilter filter = new ImageFilter();
                    
                    File directory = file.getParentFile();
                    String[] images = directory.list(filter);
                    
                    if (images.length >= 1) {
                        String upperCaseName = images[0].toUpperCase();
                        String contentType = "";
                        if (upperCaseName.endsWith(".JPG") ||
                                upperCaseName.endsWith(".JPEG")) {
                            contentType = "image/jpeg";
                        } else if (upperCaseName.endsWith(".PNG")) {
                            contentType = "image/png";
                        } else if (upperCaseName.endsWith(".GIF")) {
                            contentType = "image/gif";
                        }
                        
                        os.writeBytes("HTTP/1.1 200 \r\n");
                        os.writeBytes("Content-type: " + contentType + "\r\n");
                        os.writeBytes("Connection: close\r\n");
                        os.writeBytes("\r\n");
                        
                        try {
                            
                            byte outByte[] = new byte[1024];
                            FileInputStream fis = new FileInputStream(new
                                    File(directory.getPath(), images[0]));
                            
                            
                            int j;
                            int size = fis.available();
                            
                            for (j=0; j < size; ) {
                                
                                if (1024 > (size - j)) {
                                    
                                    fis.read(outByte, 0, (size - j - 1));
                                } else {
                                    
                                    fis.read( outByte, 0, 1024);
                                    
                                }
                                
                                os.write(outByte);
                                
                                j = j + 1024;
                                
                            }
                            fis.close();
                            
                        } catch (Exception e) {
                            
                            System.err.println(e);
                            
                        }
                        
                    }
                    
                }
                
            } catch (Exception e) {
            	System.err.println(e);
            }
            
            try {
                
                os.writeBytes("HTTP/1.1 200 \r\n");
                os.writeBytes("Content-type: " +
                        Options.getDefaultImageMimeType() + "\r\n");
                os.writeBytes("Connection: close\r\n");
                
                os.writeBytes("\r\n");
                Options.outputDefaultImage(os);
                
            } catch (Exception e) {
                
                System.err.println(e);
                
            }
            
        }
        
    }

}