package git

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"

	git2go "github.com/libgit2/git2go/v31"
)

type Fetcher struct {
	Logger   *log.Logger
	Keychain GitKeychain
}

func (f Fetcher) Fetch(dir, gitURL, gitRevision, metadataDir string) error {
	f.Logger.Printf("Cloning %q @ %q...", gitURL, gitRevision)

	repository, err := git2go.InitRepository(dir, false)
	if err != nil {
		return err
	}
	fmt.Println("remote create")

	remote2, err := repository.Remotes.CreateWithOptions(gitURL, &git2go.RemoteCreateOptions{
		Name:  "origin",
		Flags: git2go.RemoteCreateSkipInsteadof,
	})
	if err != nil {
		return err
	}

	err = remote2.Fetch([]string{"refs/*:refs/*"}, &git2go.FetchOptions{
		DownloadTags: git2go.DownloadTagsNone,
		RemoteCallbacks: git2go.RemoteCallbacks{
			CredentialsCallback: func(url string, username_from_url string, allowed_types git2go.CredentialType) (*git2go.Credential, error) {

				return nil, nil
			},
			CertificateCheckCallback: func(cert *git2go.Certificate, valid bool, hostname string) git2go.ErrorCode {
				return git2go.ErrorCodeOK
			},
		},
	}, "")
	if err != nil {
		return err
	}

	oid, err := resolveRevision(repository, gitRevision)
	if err != nil {
		return err
	}

	commit, err := repository.LookupCommit(oid)
	if err != nil {
		return err
	}

	err = repository.SetHeadDetached(commit.Id())
	if err != nil {
		return err
	}
	err = repository.CheckoutHead(&git2go.CheckoutOpts{
		Strategy: git2go.CheckoutForce,
	})
	if err != nil {
		return err
	}

	projectMetadataFile, err := os.Create(path.Join(metadataDir, "project-metadata.toml"))
	if err != nil {
		return errors.Wrapf(err, "invalid metadata destination '%s/project-metadata.toml' for git repository: %s", metadataDir, gitURL)
	}
	defer projectMetadataFile.Close()

	projectMd := project{
		Source: source{
			Type: "git",
			Metadata: metadata{
				Repository: gitURL,
				Revision:   gitRevision,
			},
			Version: version{
				Commit: commit.Id().String(),
			},
		},
	}
	if err := toml.NewEncoder(projectMetadataFile).Encode(projectMd); err != nil {
		return errors.Wrapf(err, "invalid metadata destination '%s/project-metadata.toml' for git repository: %s", metadataDir, gitRevision)
	}

	f.Logger.Printf("Successfully cloned %q @ %q in path %q", gitURL, gitRevision, dir)
	return nil
}

func resolveRevision(repository *git2go.Repository, gitRevision string) (*git2go.Oid, error) {
	ref, err := repository.References.Dwim(gitRevision)
	if err != nil {
		return git2go.NewOid(gitRevision) //TODO proper error handling
	}

	return ref.Target(), nil
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, srcInfo.Mode())
}

func copyDir(src string, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	fileInfos, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	for _, fileInfo := range fileInfos {
		srcPath := path.Join(src, fileInfo.Name())
		destPath := path.Join(dest, fileInfo.Name())

		if fileInfo.IsDir() {
			if err = copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err = copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

type project struct {
	Source source `toml:"source"`
}

type source struct {
	Type     string   `toml:"type"`
	Metadata metadata `toml:"metadata"`
	Version  version  `toml:"version"`
}

type metadata struct {
	Repository string `toml:"repository"`
	Revision   string `toml:"revision"`
}

type version struct {
	Commit string `toml:"commit"`
}
