function roundtrip(infile, outfile, createdfile)
%ROUNDTRIP Cross-validate go-sofa against the SOFA Toolbox (PLAN.md D2).
%
%   roundtrip(INFILE, OUTFILE, CREATEDFILE)
%
%   1. Loads INFILE, a SimpleFreeFieldHRIR file written by go-sofa, with
%      SOFAload and writes it back with SOFAsave to OUTFILE. The Go side
%      then checks that Data.IR, SourcePosition and ListenerPosition in
%      OUTFILE are bit-for-bit those of INFILE.
%   2. Builds a SimpleFreeFieldHRIR object from scratch with the toolbox,
%      fills it with the values toolboxValues() defines, and saves it to
%      CREATEDFILE for go-sofa to read and check against the same formula
%      (internal/interop/toolbox, `check-created`).
%
%   Runs in MATLAB and GNU Octave (with the netcdf package). The SOFA
%   Toolbox (https://github.com/sofacoustics/SOFAtoolbox) must be on the
%   path, or its SOFAtoolbox/ directory given in the environment variable
%   SOFATOOLBOX. See README.md, "Cross-validation", for the full sequence.

if exist('OCTAVE_VERSION', 'builtin')
  pkg load netcdf
end
tb = getenv('SOFATOOLBOX');
if ~isempty(tb)
  addpath(tb);
end
SOFAstart('silent');
fprintf('SOFA Toolbox %s (SOFA %s)\n', SOFAgetVersion('API'), SOFAgetVersion('SOFA'));

% 1. go-sofa -> toolbox
Obj = SOFAload(infile);
fprintf('loaded %s: %s %s, M=%d R=%d N=%d\n', infile, Obj.GLOBAL_SOFAConventions, ...
  Obj.GLOBAL_SOFAConventionsVersion, Obj.API.M, Obj.API.R, Obj.API.N);
SOFAsave(outfile, Obj);
fprintf('saved %s\n', outfile);

% 2. toolbox -> go-sofa
[M, R, N] = deal(5, 2, 16);
Obj = SOFAgetConventions('SimpleFreeFieldHRIR');
[ir, src, lst] = toolboxValues(M, R, N);
Obj.Data.IR = ir;
Obj.Data.SamplingRate = 48000;
Obj.SourcePosition = src;
Obj.ListenerPosition = lst;
Obj.GLOBAL_Title = 'SOFA Toolbox cross-validation file';
Obj.GLOBAL_AuthorContact = 'go-sofa maintainers';
Obj.GLOBAL_Organization = 'go-sofa';
Obj.GLOBAL_License = 'MIT';
Obj = SOFAupdateDimensions(Obj);
SOFAsave(createdfile, Obj);
fprintf('created %s: M=%d R=%d N=%d\n', createdfile, M, R, N);
end

function [ir, src, lst] = toolboxValues(M, R, N)
% Values with no short binary representation (thirds, sevenths, square
% roots) so that any rounding on the way shows up in a bit-exact compare.
% internal/interop/toolbox computes the same formula in Go.
ir = zeros(M, R, N);
for m = 1:M
  for r = 1:R
    for n = 1:N
      ir(m, r, n) = ((m - 1) * 100 + (r - 1) * 10 + (n - 1)) / 3 - sqrt(n) / 7;
    end
  end
end
src = zeros(M, 3);
for m = 1:M
  src(m, :) = [(m - 1) * 72 / 7, 10 / 3 - (m - 1), 1.2 + (m - 1) / 10];
end
lst = [0.1, 0.2 / 3, sqrt(2)];
end
